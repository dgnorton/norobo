package addon

type Result interface {
	Name() string
	Error() error
	SpamConfidence() float64
	SpamDescription() string
}
