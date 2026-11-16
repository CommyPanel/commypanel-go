package commypanel

import "regexp"

var (
	// App names ("slugs") are lower case letters, digits and hyphens.
	slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	// Variable names follow the shell's rule.
	envKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
)

func checkSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return ErrInvalidInput
	}

	return nil
}

func checkEnvKey(key string) error {
	if !envKeyPattern.MatchString(key) {
		return ErrInvalidInput
	}

	return nil
}
