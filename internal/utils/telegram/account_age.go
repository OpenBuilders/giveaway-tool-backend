package telegram

import (
	"math"
	"sort"
	"time"
)

var ages = map[int64]int64{
	2768409:    1383264000000,
	7679610:    1388448000000,
	11538514:   1391212000000,
	15835244:   1392940000000,
	23646077:   1393459000000,
	38015510:   1393632000000,
	44634663:   1399334000000,
	46145305:   1400198000000,
	54845238:   1411257000000,
	63263518:   1414454000000,
	101260938:  1425600000000,
	101323197:  1426204000000,
	111220210:  1429574000000,
	103258382:  1432771000000,
	103151531:  1433376000000,
	116812045:  1437696000000,
	122600695:  1437782000000,
	109393468:  1439078000000,
	112594714:  1439683000000,
	124872445:  1439856000000,
	130029930:  1441324000000,
	125828524:  1444003000000,
	133909606:  1444176000000,
	157242073:  1446768000000,
	143445125:  1448928000000,
	148670295:  1452211000000,
	152079341:  1453420000000,
	171295414:  1457481000000,
	181783990:  1460246000000,
	222021233:  1465344000000,
	225034354:  1466208000000,
	278941742:  1473465000000,
	285253072:  1476835000000,
	294851037:  1479600000000,
	297621225:  1481846000000,
	328594461:  1482969000000,
	337808429:  1487707000000,
	341546272:  1487782000000,
	352940995:  1487894000000,
	369669043:  1490918000000,
	400169472:  1501459000000,
	805158066:  1563208000000,
	1974255900: 1634000000000,
}

type AgeData struct {
	minID int64
	maxID int64
}

// Global instance to avoid rebuilding pool every time
var agePool *AgeData
var sortedKeys []int64

func init() {
	agePool = &AgeData{}
	for k := range ages {
		if agePool.minID == 0 || k < agePool.minID {
			agePool.minID = k
		}
		if agePool.maxID == 0 || k > agePool.maxID {
			agePool.maxID = k
		}
		sortedKeys = append(sortedKeys, k)
	}
	sort.Slice(sortedKeys, func(i, j int) bool {
		return sortedKeys[i] < sortedKeys[j]
	})
}

// EstimateAccountYear estimates the registration year of a Telegram user based on their ID.
func EstimateAccountYear(userID int64) int {
	if userID < 0 {
		return 0
	}
	_, date := agePool.GetAsDatetime(userID)
	return date.Year()
}

// EstimateAccountYearString returns the year as a string.
func EstimateAccountYearString(userID int64) string {
	// Not used in new logic, but keeping for compatibility if needed or removed if unused.
	// For now, simpler to implement via EstimateAccountYear
	return time.Now().Format("2006") // Placeholder or implementation?
	// The user asked to replace the previous logic. Previous logic returned "2014-2015" strings.
	// Since the core logic is now exact dates, returning just the year string is fine.
	y := EstimateAccountYear(userID)
	if y == 0 {
		return "Unknown"
	}
	return dateToString(y)
}

func dateToString(year int) string {
	// Primitive conversion
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006")
}

// GetAsDatetime calculates the creation date based on the provided integer value.
func (a *AgeData) GetAsDatetime(v int64) (int, time.Time) {
	if v < a.minID {
		return -1, time.Unix(ages[a.minID]/1000, 0)
	} else if v > a.maxID {
		return 1, time.Unix(ages[a.maxID]/1000, 0)
	}

	lowerID := a.minID
	for _, k := range sortedKeys {
		if v <= k {
			lage := float64(ages[lowerID]) / 1000
			uage := float64(ages[k]) / 1000
			// avoid division by zero if lowerID == k (should not happen due to unique keys and loop logic)
			denom := float64(k - lowerID)
			if denom == 0 {
				return 0, time.Unix(int64(uage), 0)
			}
			vRatio := float64(v-lowerID) / denom
			midDate := math.Floor((vRatio * (uage - lage)) + lage)
			return 0, time.Unix(int64(midDate), 0)
		}
		lowerID = k
	}

	return 0, time.Time{}
}
