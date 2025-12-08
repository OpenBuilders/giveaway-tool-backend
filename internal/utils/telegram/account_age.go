package telegram

import (
	"strconv"
)

// IDRange defines a range of IDs corresponding to a specific registration year.
type IDRange struct {
	Start int64
	End   int64
	Year  int
}

var idRanges = []IDRange{
	{1, 52481424, 2013},
	{52481425, 112594714, 2014},
	{112594715, 260972612, 2015},
	{260972613, 417043996, 2016},
	{417043997, 619538293, 2017},
	{619538294, 851921979, 2018},
	{851921980, 1213088939, 2019},
	{1213088940, 1974255900, 2020},
	{1974255901, 3293345067, 2021},
	{3293345068, 5171460881, 2022},
	{5171460882, 7320000000, 2023},
	{7320000001, 9223372036854775807, 2024},
}

// EstimateAccountYear estimates the registration year of a Telegram user based on their ID.
// Returns 0 if the ID is invalid (negative).
// Returns 2025 for IDs higher than the last known range (implied by "everything higher is 2025").
func EstimateAccountYear(userID int64) int {
	if userID < 0 {
		return 0
	}

	for _, r := range idRanges {
		if userID >= r.Start && userID <= r.End {
			return r.Year
		}
	}

	// The prompt says "everything higher is 2025".
	// The last range covers up to max int64 (9223372036854775807).
	// So theoretically this part is unreachable unless we update ranges,
	// but logically if it exceeds known ranges it's likely very new.
	// Since 2024 range ends at max int64, let's strictly follow the ranges.
	// However, if the user provides a new range list in the future that doesn't cover max int64,
	// this fallback is useful.
	// Given the provided ranges end at MaxInt64 with 2024,
	// there is actually no "higher" than MaxInt64 for int64.
	// Wait, checking the prompt again: "{7320000001, 9223372036854775807, 2024}, all above is 2025".
	// Since 9223372036854775807 is MaxInt64, "above" isn't possible in int64.
	// Perhaps the user meant if I modify the last range?
	// Or maybe the user meant "current year" logic.
	// But let's stick to the ranges.
	// Actually, if I look closely at the provided ranges, the last one IS 2024.
	// And the user text says "everything above is 2025".
	// Since the last range goes to MaxInt64, effectively 2025 is impossible to reach with int64 here.
	// I will just implement the loop.

	// If by some chance we have gaps or future updates:
	if userID > idRanges[len(idRanges)-1].End {
		return 2025
	}

	return 0
}

// EstimateAccountYearString returns the year as a string.
func EstimateAccountYearString(userID int64) string {
	y := EstimateAccountYear(userID)
	if y == 0 {
		return "Unknown"
	}
	return strconv.Itoa(y)
}
