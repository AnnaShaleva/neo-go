package result

// FindStorage represents the result of `findstorage` RPC handler.
type FindStorage struct {
	Results []KeyValue `json:"results"`
	// Next is an exclusive cursor for the next page: the last returned key (or
	// empty if there are no results). Pass it as `start` if Truncated is set.
	Next      []byte `json:"next"`
	Truncated bool   `json:"truncated"`
}
