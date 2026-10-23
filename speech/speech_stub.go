//go:build (!js && !android && !ios && !onix_mobile_test) || wechat

package speech

type unavailableSynthesizer struct{}

func newPlatformSynthesizer() platformSynthesizer {
	return unavailableSynthesizer{}
}

func (unavailableSynthesizer) Available() bool             { return false }
func (unavailableSynthesizer) Speak(string, Options) error { return ErrUnavailable }
func (unavailableSynthesizer) Pause() error                { return ErrUnavailable }
func (unavailableSynthesizer) Resume() error               { return ErrUnavailable }
func (unavailableSynthesizer) Stop() error                 { return ErrUnavailable }
func (unavailableSynthesizer) Status() Status              { return Status{Err: ErrUnavailable} }
func (unavailableSynthesizer) Close() error                { return nil }
