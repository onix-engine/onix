//go:build (!js && !android && !ios && !onix_mobile_test) || wechat

package camera

func platformAvailable() bool {
	return false
}

func open(Options) (frameSource, error) {
	return nil, ErrUnavailable
}
