package application

// DefaultArchiveQuery is the unfiltered archive list query used by the workbench.
func DefaultArchiveQuery() ArchiveQuery {
	return ArchiveQuery{Page: 1, PageSize: 20}
}
