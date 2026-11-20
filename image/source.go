package image

import "ownpreview/internal/utils"

type Source interface {
	Source()
	Version() string
}

type GitSource struct {
	URL string
	Ref string
}

func (g *GitSource) Source() {}

func (g *GitSource) Version() string {
	v, _ := utils.SmallAlphaNumericHash(g.URL+g.Ref, 8)
	return v
}
