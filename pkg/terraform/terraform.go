package terraform

const (
	// ExitPass is status code zero
	ExitPass int = iota

	// ExitFail is status code non-zero
	ExitFail

	StartOfOutsideBlock = "Note: Objects have changed outside of Terraform"
	EndOfOutsideBlock   = "Unless you have made equivalent changes to your configuration"
	StartOfChangeBlock  = "Terraform will perform the following actions:"
	EndOfChangeBlock    = "Plan: "
	StartOfWarningBlock = "Warning:"
	EndOfWarningBlock   = "─────"
	NoChanges           = "No changes. Infrastructure is up-to-date."
)
