package diag

// Counts summarizes a diagnostic set for the run summary and the exit code.
type Counts struct {
	Errors   int
	Warnings int
}

// Total returns the number of diagnostics that will be shown.
func (c Counts) Total() int {
	return c.Errors + c.Warnings
}

// Summarize tallies diagnostics by severity.
func Summarize(ds []Diagnostic) Counts {
	var c Counts
	for _, d := range ds {
		switch d.Severity {
		case Error:
			c.Errors++
		case Warning:
			c.Warnings++
		}
	}
	return c
}
