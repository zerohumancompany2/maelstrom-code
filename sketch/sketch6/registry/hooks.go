package registry

type IdentityHook struct{}

func (IdentityHook) Name() string { return "identity" }

func (IdentityHook) Apply(input []byte) ([]byte, error) {
	return input, nil
}
