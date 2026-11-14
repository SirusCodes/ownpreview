package image

type Source interface {
	Source()
}

type GitSource struct {
	URL string
	Ref string
}

func (g *GitSource) Source() {}
