package dashboard

// Only fields intentionally exposed to the browser belong in these DTOs.
type Request struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type Props struct {
	Requests []Request `json:"requests"`
}
