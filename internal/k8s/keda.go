package k8s

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"helm.sh/helm/v4/pkg/action"
	chartv1 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/repo/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
)

const (
	kedaNamespace = "keda"
	kedaRepoName  = "kedacore"
	kedaRepoURL   = "https://kedacore.github.io/charts"
)

func IsKEDASetup(settings *cli.EnvSettings) (bool, error) {
	kedaDeployed, err := ReleaseDeployed(settings, kedaNamespace, "keda")
	if err != nil {
		return false, fmt.Errorf("check keda release: %w", err)
	}
	if !kedaDeployed {
		return false, nil
	}

	httpAddonDeployed, err := ReleaseDeployed(settings, kedaNamespace, "http-add-on")
	if err != nil {
		return false, fmt.Errorf("check http-add-on release: %w", err)
	}

	return httpAddonDeployed, nil
}

func ReleaseDeployed(settings *cli.EnvSettings, namespace, releaseName string) (bool, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER")); err != nil {
		return false, err
	}

	statusClient := action.NewStatus(actionConfig)
	rel, err := statusClient.Run(releaseName)
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return releaseIsDeployed(rel)
}

func releaseIsDeployed(rel any) (bool, error) {
	r, ok := rel.(*releasev1.Release)
	if !ok || r == nil {
		return false, fmt.Errorf("unexpected release type %T", rel)
	}
	return r.Info != nil && r.Info.Status == common.StatusDeployed, nil
}

func SetupKeda(ctx context.Context, client *kubernetes.Clientset, kubeConfPath, namespace, wildcardDomain string) error {
	set := cli.New()
	if kubeConfPath != "" {
		set.KubeConfig = kubeConfPath
	}

	ready, err := IsKEDASetup(set)
	if err != nil {
		return err
	}

	if !ready {
		ns := corev1.Namespace{
			Name: kedaNamespace,
		}

		_, err := client.CoreV1().Namespaces().Create(ctx, &ns, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}

		// 2. Add and update the kedacore Helm repository
		if err := ensureHelmRepo(set, kedaRepoName, kedaRepoURL); err != nil {
			return fmt.Errorf("setup helm repo: %w", err)
		}

		// 3. Install KEDA Core if not deployed
		kedaDeployed, err := ReleaseDeployed(set, kedaNamespace, "keda")
		if err != nil {
			return err
		}
		if !kedaDeployed {
			if err := installChart(set, kedaNamespace, "keda", "kedacore/keda", nil); err != nil {
				return fmt.Errorf("install keda core: %w", err)
			}
		}

		// 4. Install KEDA HTTP Add-on if not deployed
		httpDeployed, err := ReleaseDeployed(set, kedaNamespace, "http-add-on")
		if err != nil {
			return err
		}
		if !httpDeployed {
			if err := installChart(set, kedaNamespace, "http-add-on", "kedacore/keda-add-ons-http", nil); err != nil {
				return fmt.Errorf("install keda http-add-on: %w", err)
			}
		}
	}

	// 5. Always ensure the wildcard Ingress exists (idempotent)
	if err := ensureWildcardIngress(ctx, client, wildcardDomain); err != nil {
		return fmt.Errorf("create wildcard ingress: %w", err)
	}

	return nil
}

// UpgradeKEDA updates the kedacore Helm repository index and upgrades both KEDA Core and the HTTP Add-on.
func UpgradeKEDA(kubeconfigPath string, kedaVals, httpAddonVals map[string]any) error {
	settings := cli.New()
	if kubeconfigPath != "" {
		settings.KubeConfig = kubeconfigPath
	}
	settings.SetNamespace(kedaNamespace)

	// Refresh chart index so LocateChart fetches the latest chart version
	if err := ensureHelmRepo(settings, kedaRepoName, kedaRepoURL); err != nil {
		return fmt.Errorf("update helm repo: %w", err)
	}

	if err := upgradeChart(settings, kedaNamespace, "keda", "kedacore/keda", kedaVals); err != nil {
		return fmt.Errorf("upgrade keda core: %w", err)
	}

	if err := upgradeChart(settings, kedaNamespace, "http-add-on", "kedacore/keda-add-ons-http", httpAddonVals); err != nil {
		return fmt.Errorf("upgrade keda http-add-on: %w", err)
	}

	return nil
}

func ensureHelmRepo(settings *cli.EnvSettings, name, url string) error {
	e := &repo.Entry{Name: name, URL: url}
	r, err := repo.NewChartRepository(e, getter.All(settings))
	if err != nil {
		return err
	}

	if _, err := r.DownloadIndexFile(); err != nil {
		return err
	}

	f, err := loadRepoFile(settings.RepositoryConfig)
	if err != nil {
		return err
	}
	f.Update(e)
	return f.WriteFile(settings.RepositoryConfig, 0644)
}

func loadRepoFile(path string) (*repo.File, error) {
	f, err := repo.LoadFile(path)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return repo.NewFile(), nil
}

func prepareHelmAction(settings *cli.EnvSettings, namespace, chartRef string) (*action.Configuration, *chartv1.Chart, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(settings.RESTClientGetter(), namespace, os.Getenv("HELM_DRIVER")); err != nil {
		return nil, nil, fmt.Errorf("init helm action config: %w", err)
	}

	cpOptions := action.ChartPathOptions{}
	chartPath, err := cpOptions.LocateChart(chartRef, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("locate chart %s: %w", chartRef, err)
	}

	chrt, err := loader.Load(chartPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load chart %s: %w", chartPath, err)
	}

	return actionConfig, chrt, nil
}

func installChart(settings *cli.EnvSettings, namespace, releaseName, chartRef string, vals map[string]any) error {
	actionConfig, chrt, err := prepareHelmAction(settings, namespace, chartRef)
	if err != nil {
		return err
	}

	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.Replace = true
	install.WaitForJobs = true
	install.Timeout = 5 * time.Minute
	install.WaitStrategy = kube.StatusWatcherStrategy

	_, err = install.Run(chrt, vals)
	return err
}

func upgradeChart(settings *cli.EnvSettings, namespace, releaseName, chartRef string, vals map[string]any) error {
	deployed, err := ReleaseDeployed(settings, namespace, releaseName)
	if err != nil {
		return err
	}
	if !deployed {
		return installChart(settings, namespace, releaseName, chartRef, vals)
	}

	actionConfig, chrt, err := prepareHelmAction(settings, namespace, chartRef)
	if err != nil {
		return err
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = namespace
	upgrade.WaitForJobs = true
	upgrade.Timeout = 5 * time.Minute

	_, err = upgrade.Run(releaseName, chrt, vals)
	return err
}

func ensureWildcardIngress(ctx context.Context, clientset *kubernetes.Clientset, wildcardDomain string) error {
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		Name:      "ownpreview-wildcard",
		Namespace: kedaNamespace,
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: fmt.Sprintf("*.%s", wildcardDomain),
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     "/",
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "keda-add-ons-http-interceptor-proxy",
										Port: networkingv1.ServiceBackendPort{
											Number: 8080,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := clientset.NetworkingV1().Ingresses(kedaNamespace).Create(ctx, ingress, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}
