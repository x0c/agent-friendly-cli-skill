// All runtime messages of this package are declared centrally here: rewording
// a message only ever touches this file; localization only swaps this file's
// contents.
package profile

var (
	msgInvalidProfileName     = "invalid profile name: only 1-64 characters of letters, digits, dots, underscores and hyphens are allowed"
	msgServerMustBeFullHTTP   = "the %s server address must be a full http or https URL"
	msgUsernameRequired       = "--username is required"
	msgPasswordMissing        = "no password provided; set %s or enter it in the terminal"
	msgHomeDirUnknown         = "cannot determine the user home directory: %w"
	msgProfileNotFound        = "profile %q not found"
	msgProfileContentMismatch = "profile file content mismatch: %s"
	msgProfileDirUnsafe       = "profile directory is unsafe: %s"
	msgProfileDirHasSymlink   = "symbolic link found in the profile directory: %s"
	msgConfigDirPerm0700      = "config directory %s must have permissions exactly 0700"
	msgProfileFilePerm0600    = "profile file %s must have permissions exactly 0600"
)
