package deploy

import (
	"context"
	"ownpreview/image"
)

type Deployer interface {
	Deploy(ctx context.Context, image string, registry image.Registry) error
}
