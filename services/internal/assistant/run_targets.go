package assistant

// Discovery hints come only from exact receipts. They help resolve references
// in a later request; they are not a compiled or verified history of goals.
func forgetRemovedTargets(targets, selections map[string]string, removed []string) {
	for _, id := range removed {
		for role, target := range targets {
			if target == id {
				delete(targets, role)
				delete(selections, role)
			}
		}
	}
}
