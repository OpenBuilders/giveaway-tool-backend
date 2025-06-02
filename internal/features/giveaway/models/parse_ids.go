package models

// ParseIDsResponse представляет результат парсинга файла с ID
type ParseIDsResponse struct {
	TotalIDs int      `json:"total_ids"` // Общее количество уникальных ID
	IDs      []string `json:"ids"`       // Список уникальных ID
}
