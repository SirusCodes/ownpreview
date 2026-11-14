package image

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
	return g.Ref
}
