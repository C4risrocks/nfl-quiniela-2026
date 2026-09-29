package espn

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nfl-quiniela-2026/db"
)

var (
	logoCodeRegex    = regexp.MustCompile(`/(?:[0-9]+-dark/|[0-9]+/)?([a-z0-9]+)\.png`)
	fpiCacheMu       sync.Mutex
	fpiProbsCache    []*db.TeamPlayoffProbability
	fpiMatchupsCache []*db.PlayoffMatchupImpact
	fpiCacheExpiry   time.Time
)

// Embedded raw fallback JSON from ESPN Creative Dev for 100% offline resilience
const defaultAFCHeatmapJSON = `{"data": [{"abbrev": "buf", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/buf.png", "values": ["0.967", "0.898", "0.331", "0.069"]}, {"abbrev": "kc", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/kc.png", "values": ["0.954", "0.789", "0.273", "0.165"]}, {"abbrev": "jac", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/jac.png", "values": ["0.908", "0.813", "0.118", "0.095"]}, {"abbrev": "bal", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/bal.png", "values": ["0.881", "0.559", "0.143", "0.322"]}, {"abbrev": "cin", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/cin.png", "values": ["0.802", "0.352", "0.078", "0.45"]}, {"abbrev": "den", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/den.png", "values": ["0.586", "0.131", "0.028", "0.455"]}, {"abbrev": "ne", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ne.png", "values": ["0.376", "0.084", "0.008", "0.292"]}, {"abbrev": "lv", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/lv.png", "values": ["0.384", "0.074", "0.01", "0.31"]}, {"abbrev": "pit", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/pit.png", "values": ["0.336", "0.074", "0.006", "0.262"]}, {"abbrev": "ind", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ind.png", "values": ["0.286", "0.105", "0.003", "0.181"]}, {"abbrev": "hou", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/hou.png", "values": ["0.224", "0.08", "0.001", "0.144"]}, {"abbrev": "cle", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/cle.png", "values": ["0.106", "0.014", "0.001", "0.092"]}, {"abbrev": "nyj", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/nyj.png", "values": ["0.128", "0.018", "0.001", "0.11"]}, {"abbrev": "lac", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/lac.png", "values": ["0.054", "0.006", "0", "0.048"]}, {"abbrev": "ten", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ten.png", "values": ["0.007", "0.002", "0", "0.005"]}, {"abbrev": "mia", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/mia.png", "values": ["0", "0", "0", "0"]}], "updated_at": "09/29/26, 9:47 A.M. ET", "is_live": true}`
const defaultNFCHeatmapJSON = `{"data": [{"abbrev": "sf", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/sf.png", "values": ["0.983", "0.786", "0.597", "0.198"]}, {"abbrev": "sea", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/sea.png", "values": ["0.746", "0.162", "0.098", "0.585"]}, {"abbrev": "chi", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/chi.png", "values": ["0.706", "0.275", "0.058", "0.432"]}, {"abbrev": "det", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/det.png", "values": ["0.764", "0.354", "0.09", "0.41"]}, {"abbrev": "min", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/min.png", "values": ["0.753", "0.348", "0.082", "0.405"]}, {"abbrev": "dal", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/dal.png", "values": ["0.676", "0.582", "0.029", "0.094"]}, {"abbrev": "la", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/la.png", "values": ["0.557", "0.052", "0.029", "0.505"]}, {"abbrev": "no", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/no.png", "values": ["0.565", "0.545", "0.005", "0.02"]}, {"abbrev": "phi", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/phi.png", "values": ["0.393", "0.293", "0.006", "0.1"]}, {"abbrev": "wsh", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/wsh.png", "values": ["0.19", "0.108", "0.002", "0.082"]}, {"abbrev": "gb", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/gb.png", "values": ["0.133", "0.024", "0.002", "0.109"]}, {"abbrev": "car", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/car.png", "values": ["0.277", "0.254", "0.001", "0.023"]}, {"abbrev": "atl", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/atl.png", "values": ["0.133", "0.118", "0", "0.015"]}, {"abbrev": "ari", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ari.png", "values": ["0.011", "0", "0", "0.01"]}, {"abbrev": "tb", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/tb.png", "values": ["0.088", "0.083", "0", "0.005"]}, {"abbrev": "nyg", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/nyg.png", "values": ["0.025", "0.016", "0", "0.008"]}], "updated_at": "09/29/26, 9:47 A.M. ET", "is_live": true}`
const defaultMatchupsJSON = `{"week": "4", "matchups": [{"away": {"name": "Steelers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/pit.png", "values": ["0.55", "0.34", "0.43", "0.22"]}, "home": {"name": "Browns", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/cle.png", "values": ["0.45", "0.11", "0.17", "0.06"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-01T20:15"}, {"away": {"name": "Colts", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ind.png", "values": ["0.54", "0.29", "0.35", "0.20"]}, "home": {"name": "Commanders", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/wsh.png", "values": ["0.45", "0.19", "0.26", "0.13"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T09:30"}, {"away": {"name": "Cardinals", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ari.png", "values": ["0.54", "0.01", "0.02", "0.00"]}, "home": {"name": "Giants", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/nyg.png", "values": ["0.45", "0.02", "0.04", "0.01"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Cowboys", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/dal.png", "values": ["0.54", "0.68", "0.74", "0.60"]}, "home": {"name": "Texans", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/hou.png", "values": ["0.45", "0.22", "0.30", "0.16"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Packers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/gb.png", "values": ["0.67", "0.13", "0.16", "0.07"]}, "home": {"name": "Buccaneers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/tb.png", "values": ["0.32", "0.09", "0.14", "0.06"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Jaguars", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/jac.png", "values": ["0.55", "0.91", "0.95", "0.86"]}, "home": {"name": "Bengals", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/cin.png", "values": ["0.45", "0.80", "0.88", "0.74"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Rams", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/la.png", "values": ["0.64", "0.56", "0.64", "0.40"]}, "home": {"name": "Eagles", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/phi.png", "values": ["0.35", "0.39", "0.53", "0.32"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Patriots", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ne.png", "values": ["0.29", "0.38", "0.52", "0.32"]}, "home": {"name": "Bills", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/buf.png", "values": ["0.71", "0.97", "0.98", "0.93"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Jets", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/nyj.png", "values": ["0.37", "0.13", "0.19", "0.09"]}, "home": {"name": "Bears", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/chi.png", "values": ["0.63", "0.71", "0.76", "0.62"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Titans", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/ten.png", "values": ["0.16", "0.01", "0.03", "0.00"]}, "home": {"name": "Ravens", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/bal.png", "values": ["0.84", "0.88", "0.90", "0.79"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T13:00"}, {"away": {"name": "Dolphins", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/mia.png", "values": ["0.17", "0.00", "0.00", "0.00"]}, "home": {"name": "Vikings", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/min.png", "values": ["0.83", "0.75", "0.78", "0.63"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T16:05"}, {"away": {"name": "Broncos", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/den.png", "values": ["0.23", "0.59", "0.71", "0.55"]}, "home": {"name": "49ers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/sf.png", "values": ["0.77", "0.98", "0.99", "0.97"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T16:25"}, {"away": {"name": "Chiefs", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/kc.png", "values": ["0.70", "0.95", "0.97", "0.91"]}, "home": {"name": "Raiders", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/lv.png", "values": ["0.30", "0.38", "0.54", "0.32"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T16:25"}, {"away": {"name": "Chargers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/lac.png", "values": ["0.26", "0.05", "0.09", "0.04"]}, "home": {"name": "Seahawks", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/sea.png", "values": ["0.74", "0.75", "0.78", "0.65"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-04T16:25"}, {"away": {"name": "Lions", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/det.png", "values": ["0.61", "0.76", "0.83", "0.66"]}, "home": {"name": "Panthers", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/car.png", "values": ["0.39", "0.28", "0.36", "0.22"]}, "indicators": {"away": [0], "home": []}, "datetime": "2026-10-04T20:20"}, {"away": {"name": "Falcons", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/atl.png", "values": ["0.37", "0.13", "0.22", "0.08"]}, "home": {"name": "Saints", "logo": "https://a.espncdn.com/combiner/i?img=/i/teamlogos/nfl/500-dark/no.png", "values": ["0.63", "0.56", "0.65", "0.42"]}, "indicators": {"away": [], "home": [0]}, "datetime": "2026-10-05T20:15"}]}`

type rawHeatmapResponse struct {
	Data []struct {
		Abbrev string   `json:"abbrev"`
		Logo   string   `json:"logo"`
		Values []string `json:"values"`
	} `json:"data"`
	UpdatedAt string `json:"updated_at"`
	IsLive    bool   `json:"is_live"`
}

type rawMatchupsResponse struct {
	Week     string `json:"week"`
	Matchups []struct {
		Away struct {
			Name   string   `json:"name"`
			Logo   string   `json:"logo"`
			Values []string `json:"values"`
		} `json:"away"`
		Home struct {
			Name   string   `json:"name"`
			Logo   string   `json:"logo"`
			Values []string `json:"values"`
		} `json:"home"`
		Indicators struct {
			Away []int `json:"away"`
			Home []int `json:"home"`
		} `json:"indicators"`
		Datetime string `json:"datetime"`
	} `json:"matchups"`
}

func parseCodeFromLogo(logoURL, defaultAbbrev string) string {
	matches := logoCodeRegex.FindStringSubmatch(strings.ToLower(logoURL))
	if len(matches) > 1 {
		return NormalizeTeamCode(matches[1])
	}
	return NormalizeTeamCode(defaultAbbrev)
}

// FetchFPIPlayoffProbabilities fetches live FPI probabilities from ESPN Creative Dev, with fallback
func (c *Client) FetchFPIPlayoffProbabilities(seasonYear, weekNumber int, teamMap map[string]*db.Team) ([]*db.TeamPlayoffProbability, []*db.PlayoffMatchupImpact, error) {
	fpiCacheMu.Lock()
	if len(fpiProbsCache) > 0 && time.Now().Before(fpiCacheExpiry) {
		pCopy := make([]*db.TeamPlayoffProbability, len(fpiProbsCache))
		copy(pCopy, fpiProbsCache)
		mCopy := make([]*db.PlayoffMatchupImpact, len(fpiMatchupsCache))
		copy(mCopy, fpiMatchupsCache)
		fpiCacheMu.Unlock()
		return pCopy, mCopy, nil
	}
	fpiCacheMu.Unlock()

	// 1. Fetch raw data (HTTP or fallback)
	afcBytes := c.fetchFPIEndpoint("https://creative-dev.espn.com/nfl-probabilities/inline-story-modules/heatmap_nfl-afc.json", defaultAFCHeatmapJSON)
	nfcBytes := c.fetchFPIEndpoint("https://creative-dev.espn.com/nfl-probabilities/inline-story-modules/heatmap_nfl-nfc.json", defaultNFCHeatmapJSON)
	matchupsBytes := c.fetchFPIEndpoint("https://creative-dev.espn.com/nfl-probabilities/inline-story-modules/matchups_nfl.json", defaultMatchupsJSON)

	var rawAFC, rawNFC rawHeatmapResponse
	var rawMatchups rawMatchupsResponse

	_ = json.Unmarshal([]byte(afcBytes), &rawAFC)
	_ = json.Unmarshal([]byte(nfcBytes), &rawNFC)
	_ = json.Unmarshal([]byte(matchupsBytes), &rawMatchups)

	weekNum := 4
	if w, err := strconv.Atoi(rawMatchups.Week); err == nil && w > 0 {
		weekNum = w
	}

	// 2. Parse Matchup Impact cards and build lookup maps for team next games
	type teamMatchupLeverage struct {
		oppCode      string
		winProb      float64
		playoffWithW float64
		playoffWithL float64
		isFav        bool
	}
	leverageMap := make(map[string]teamMatchupLeverage)
	matchupImpacts := make([]*db.PlayoffMatchupImpact, 0, len(rawMatchups.Matchups))

	for _, m := range rawMatchups.Matchups {
		awayCode := parseCodeFromLogo(m.Away.Logo, "")
		homeCode := parseCodeFromLogo(m.Home.Logo, "")

		awayWinProb := 0.5
		awayCur := 0.0
		awayWin := 0.0
		awayLoss := 0.0
		if len(m.Away.Values) >= 4 {
			awayWinProb, _ = strconv.ParseFloat(m.Away.Values[0], 64)
			awayCur, _ = strconv.ParseFloat(m.Away.Values[1], 64)
			awayWin, _ = strconv.ParseFloat(m.Away.Values[2], 64)
			awayLoss, _ = strconv.ParseFloat(m.Away.Values[3], 64)
		}

		homeWinProb := 0.5
		homeCur := 0.0
		homeWin := 0.0
		homeLoss := 0.0
		if len(m.Home.Values) >= 4 {
			homeWinProb, _ = strconv.ParseFloat(m.Home.Values[0], 64)
			homeCur, _ = strconv.ParseFloat(m.Home.Values[1], 64)
			homeWin, _ = strconv.ParseFloat(m.Home.Values[2], 64)
			homeLoss, _ = strconv.ParseFloat(m.Home.Values[3], 64)
		}

		awayIsFav := len(m.Indicators.Away) > 0 || awayWinProb > homeWinProb
		homeIsFav := len(m.Indicators.Home) > 0 || homeWinProb > awayWinProb

		leverageMap[awayCode] = teamMatchupLeverage{
			oppCode:      homeCode,
			winProb:      awayWinProb,
			playoffWithW: awayWin,
			playoffWithL: awayLoss,
			isFav:        awayIsFav,
		}
		leverageMap[homeCode] = teamMatchupLeverage{
			oppCode:      awayCode,
			winProb:      homeWinProb,
			playoffWithW: homeWin,
			playoffWithL: homeLoss,
			isFav:        homeIsFav,
		}

		awaySwing := math.Abs(awayWin - awayLoss)
		homeSwing := math.Abs(homeWin - homeLoss)
		totalSwing := awaySwing + homeSwing

		stakes := "moderate"
		if totalSwing >= 0.30 {
			stakes = "critical"
		} else if totalSwing >= 0.15 {
			stakes = "high"
		}

		awayName := m.Away.Name
		awayLogo := m.Away.Logo
		if t, ok := teamMap[awayCode]; ok {
			awayName = t.City + " " + t.Name
			awayLogo = t.LogoURL
		}
		homeName := m.Home.Name
		homeLogo := m.Home.Logo
		if t, ok := teamMap[homeCode]; ok {
			homeName = t.City + " " + t.Name
			homeLogo = t.LogoURL
		}

		matchupImpacts = append(matchupImpacts, &db.PlayoffMatchupImpact{
			WeekNumber:      weekNum,
			GameDateTime:    m.Datetime,
			AwayTeamCode:    awayCode,
			AwayTeamName:    awayName,
			AwayLogoURL:     awayLogo,
			AwayWinProb:     awayWinProb,
			AwayPlayoffCur:  awayCur,
			AwayPlayoffWin:  awayWin,
			AwayPlayoffLoss: awayLoss,
			AwayIsFavorite:  awayIsFav,
			HomeTeamCode:    homeCode,
			HomeTeamName:    homeName,
			HomeLogoURL:     homeLogo,
			HomeWinProb:     homeWinProb,
			HomePlayoffCur:  homeCur,
			HomePlayoffWin:  homeWin,
			HomePlayoffLoss: homeLoss,
			HomeIsFavorite:  homeIsFav,
			StakesLevel:     stakes,
		})
	}

	// 3. Parse AFC & NFC Heatmaps
	probs := make([]*db.TeamPlayoffProbability, 0, len(rawAFC.Data)+len(rawNFC.Data))

	parseConference := func(entries []struct {
		Abbrev string   `json:"abbrev"`
		Logo   string   `json:"logo"`
		Values []string `json:"values"`
	}, confName string) {
		for _, e := range entries {
			code := parseCodeFromLogo(e.Logo, e.Abbrev)
			makePlayoffs := 0.0
			clinchDiv := 0.0
			firstSeed := 0.0
			wildCard := 0.0

			if len(e.Values) >= 4 {
				makePlayoffs, _ = strconv.ParseFloat(e.Values[0], 64)
				clinchDiv, _ = strconv.ParseFloat(e.Values[1], 64)
				firstSeed, _ = strconv.ParseFloat(e.Values[2], 64)
				wildCard, _ = strconv.ParseFloat(e.Values[3], 64)
			}

			prob := &db.TeamPlayoffProbability{
				SeasonYear:         seasonYear,
				WeekNumber:         weekNum,
				TeamCode:           code,
				Conference:         confName,
				MakePlayoffsPct:    makePlayoffs,
				ClinchDivisionPct:  clinchDiv,
				ClinchFirstSeedPct: firstSeed,
				WildCardPct:        wildCard,
				UpdatedAt:          time.Now(),
			}

			if t, ok := teamMap[code]; ok {
				prob.TeamID = t.ID
				prob.TeamName = t.Name
				prob.TeamCity = t.City
				prob.LogoURL = t.LogoURL
				prob.Division = t.Division
				if prob.Conference == "" {
					prob.Conference = t.Conference
				}
			}

			if lev, ok := leverageMap[code]; ok {
				prob.NextOpponentCode = lev.oppCode
				prob.WinProjPct = lev.winProb
				prob.PlayoffPctWithWin = lev.playoffWithW
				prob.PlayoffPctWithLoss = lev.playoffWithL
				prob.PlayoffLeverage = math.Round((lev.playoffWithW-lev.playoffWithL)*1000) / 1000
				prob.IsFavorite = lev.isFav
				if opp, ok := teamMap[lev.oppCode]; ok {
					prob.NextOpponentLogo = opp.LogoURL
				}
			}

			probs = append(probs, prob)
		}
	}

	parseConference(rawAFC.Data, "AFC")
	parseConference(rawNFC.Data, "NFC")

	sort.Slice(probs, func(i, j int) bool {
		if probs[i].MakePlayoffsPct != probs[j].MakePlayoffsPct {
			return probs[i].MakePlayoffsPct > probs[j].MakePlayoffsPct
		}
		return probs[i].ClinchDivisionPct > probs[j].ClinchDivisionPct
	})

	fpiCacheMu.Lock()
	fpiProbsCache = probs
	fpiMatchupsCache = matchupImpacts
	fpiCacheExpiry = time.Now().Add(15 * time.Minute)
	fpiCacheMu.Unlock()

	return probs, matchupImpacts, nil
}

func (c *Client) fetchFPIEndpoint(url, fallback string) string {
	if c.httpClient != nil {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0")
			req.Header.Set("Accept", "application/json")
			resp, err := c.httpClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err == nil && len(body) > 100 {
					return string(body)
				}
			}
		}
	}
	return fallback
}
