package models

type ParseIDsResponse struct {
	TotalIDs int      `json:"total_ids"`
	IDs      []string `json:"ids"`
}
