package challengeexposers

type Dns01 struct {
}

func (h *Dns01) Expose(domain string, token string, key string) error {
	return nil
}

func (h *Dns01) Remove(domain string, token string) (bool, error) {
	return true, nil
}
