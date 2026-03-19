package auth

func ValidToken(got, expected string) bool {
	return got != "" && expected != "" && got == expected
}
