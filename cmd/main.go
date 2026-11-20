package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"ownpreview/deploy"
	"ownpreview/image"
	"ownpreview/internal/k8s"
	"ownpreview/internal/utils"
	"path/filepath"

	"k8s.io/client-go/util/homedir"
)

type flags struct {
	kubeconfig string
	namespace  string
	gitURL     string
	gitRef     string
	project    string
	logLevel   string
	logFormat  string
}

func parseFlags() (*flags, error) {
	kubeconfig := flag.String("kubeconfig", "", "(optional) absolute path to the kubeconfig file")
	namespace := flag.String("namespace", "default", "Kubernetes namespace to use (default: default)")
	gitURL := flag.String("git-url", "", "Git repository URL to build the container from")
	gitRef := flag.String("git-ref", "main", "Git reference (branch, tag, or commit) to build the container from (default: main)")
	project := flag.String("project", "my-project", "Project name for the container build (default: my-project)")
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logFormat := flag.String("log-format", "text", "Log format: text, json")

	flag.Parse()

	if *kubeconfig == "" {
		home := homedir.HomeDir()
		if home == "" {
			return nil, errors.New("unable to determine home directory for kubeconfig")
		}
		*kubeconfig = filepath.Join(home, ".kube", "config")
	}

	if *gitURL == "" {
		return nil, errors.New("git-url is required")
	}

	return &flags{
		kubeconfig: *kubeconfig,
		namespace:  *namespace,
		gitURL:     *gitURL,
		gitRef:     *gitRef,
		project:    *project,
		logLevel:   *logLevel,
		logFormat:  *logFormat,
	}, nil
}

func run(ctx context.Context, f *flags, logger *slog.Logger) error {
	logger.InfoContext(ctx, "starting OwnPreview", "project", f.project, "git_ref", f.gitRef)

	logger.DebugContext(ctx, "validating Kubernetes configuration", "kubeconfig", f.kubeconfig)
	if _, err := os.Stat(f.kubeconfig); os.IsNotExist(err) {
		return fmt.Errorf("kubeconfig file not found at %s: %w", f.kubeconfig, err)
	}

	logger.DebugContext(ctx, "connecting to Kubernetes", "namespace", f.namespace)
	k8sClient, err := k8s.GetClient(&f.kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	logger.DebugContext(ctx, "Kubernetes client connected")

	logger.InfoContext(ctx, "setting up container registry", "namespace", f.namespace)
	rg := image.NewK8sRegistry(k8sClient, f.namespace)
	if err := rg.Setup(ctx); err != nil {
		return fmt.Errorf("failed to setup registry: %w", err)
	}

	logger.InfoContext(ctx, "setting up KEDA", "namespace", f.namespace)
	if err := k8s.SetupKeda(ctx, k8sClient, f.kubeconfig, f.namespace, "local.test"); err != nil {
		return fmt.Errorf("failed to setup KEDA: %w", err)
	}
	logger.DebugContext(ctx, "KEDA setup complete")

	logger.InfoContext(ctx, "building project", "project", f.project, "git_ref", f.gitRef)
	builder := image.NewK8sBuilder(k8sClient, f.namespace)
	source := &image.GitSource{
		URL: f.gitURL,
		Ref: f.gitRef,
	}

	buildRequest := image.BuildRequest{
		Source:    source,
		ImageName: f.project,
		Registry:  rg,
	}

	if err := builder.Build(ctx, buildRequest); err != nil {
		return fmt.Errorf("failed to build container: %w", err)
	}
	logger.InfoContext(ctx, "container image built successfully", "image", f.project, "version", source.Version())

	logger.DebugContext(ctx, "connecting to Kubernetes dynamic API")
	dynClient, err := k8s.GetDynamicClient(&f.kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes dynamic client: %w", err)
	}

	logger.InfoContext(ctx, "deploying application", "app", f.project, "version", source.Version())
	deployer := deploy.NewK8sDeployer(k8sClient, f.namespace, dynClient)
	deployReq := deploy.DeployRequest{
		Image:    fmt.Sprintf("%s/%s:%s", rg.GetRegistryURL(), f.project, source.Version()),
		Registry: rg,
		AppName:  f.project,
		Version:  source.Version(),
		AppPort:  8080,
		Host:     fmt.Sprintf("%s-%s.local.test", f.project, source.Version()),
	}
	if err := deployer.Deploy(ctx, deployReq); err != nil {
		return fmt.Errorf("failed to deploy application %s (%s): %w", f.project, source.Version(), err)
	}

	logger.InfoContext(ctx, "deployment complete", "host", deployReq.Host)
	return nil
}

func main() {
	f, err := parseFlags()
	if err != nil {
		// Use a temporary logger without touching default logger
		logger := utils.NewLogger("main")
		logger.Error("failed to parse flags", "error", err)
		os.Exit(1)
	}

	utils.ConfigureLogger(f.logLevel, f.logFormat)
	logger := utils.NewLogger("main")

	ctx := context.Background()
	if err := run(ctx, f, logger); err != nil {
		logger.ErrorContext(ctx, "execution failed", "error", err)
		os.Exit(1)
	}
}
