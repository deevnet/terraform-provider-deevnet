package provider_test

import "regexp"

var (
	regexpDigits = regexp.MustCompile(`^[0-9]+$`)
	regexpCIDR   = regexp.MustCompile(`^10\.[0-9]+\.[0-9]+\.0/24$`)
)
