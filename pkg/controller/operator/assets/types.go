package assets

type Data struct {
	Image                string
	TargetNamespace      string
	ClusterWide          bool
	AdditionalNamespaces []string
}

func (i Data) AllNamespaces() []string {
	all := []string{
		i.TargetNamespace,
	}
	all = append(all, i.AdditionalNamespaces...)

	return all
}
