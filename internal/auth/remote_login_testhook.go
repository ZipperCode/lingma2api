package auth

// SetUserLoginURLForTest overrides userLoginURL for the duration of a test.
// Returns the previous value so the test can restore it.
//
// NOTE: not safe for parallel tests targeting different URLs.
func SetUserLoginURLForTest(target string) string {
	old := userLoginURL
	userLoginURL = target
	return old
}

// CurrentUserLoginURLForTest returns the current userLoginURL without mutating it.
func CurrentUserLoginURLForTest() string {
	return userLoginURL
}
