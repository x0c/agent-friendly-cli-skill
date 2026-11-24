// All runtime messages of this package are declared here: to change wording,
// edit only this file; for localized distribution, only this file's contents
// change.
package secretfile

var (
	msgDirPermUnsafe     = "insecure permissions on config directory: %s must be 0700"
	msgFilePermUnsafe    = "insecure permissions on config file: %s must be 0600"
	msgDirUnsafe         = "insecure config directory: %s"
	msgFileUnsafe        = "insecure config file: %s"
	msgTightenDirFailed  = "failed to tighten config directory permissions to 0700: %s: %w"
	msgParseConfigFailed = "failed to parse config file: %w"
)
