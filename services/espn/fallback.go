package espn

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"

	"nfl-quiniela-2026/db"
)

type defaultRoster struct {
	QB   db.PlayerStatEntry
	RB   db.PlayerStatEntry
	WR1  db.PlayerStatEntry
	WR2  db.PlayerStatEntry
	DEF1 db.PlayerStatEntry
	DEF2 db.PlayerStatEntry
}

var nflStarters = map[string]defaultRoster{
	"KC": {
		QB:   db.PlayerStatEntry{Name: "Patrick Mahomes", Jersey: "15", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3139477.png"},
		RB:   db.PlayerStatEntry{Name: "Isiah Pacheco", Jersey: "10", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361529.png"},
		WR1:  db.PlayerStatEntry{Name: "Travis Kelce", Jersey: "87", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/15847.png"},
		WR2:  db.PlayerStatEntry{Name: "Rashee Rice", Jersey: "4", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429034.png"},
		DEF1: db.PlayerStatEntry{Name: "Nick Bolton", Jersey: "32", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361430.png"},
		DEF2: db.PlayerStatEntry{Name: "Chris Jones", Jersey: "95", Position: "DT", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3044859.png"},
	},
	"BAL": {
		QB:   db.PlayerStatEntry{Name: "Lamar Jackson", Jersey: "8", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3916387.png"},
		RB:   db.PlayerStatEntry{Name: "Derrick Henry", Jersey: "22", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3043078.png"},
		WR1:  db.PlayerStatEntry{Name: "Zay Flowers", Jersey: "4", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429994.png"},
		WR2:  db.PlayerStatEntry{Name: "Mark Andrews", Jersey: "89", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3116365.png"},
		DEF1: db.PlayerStatEntry{Name: "Roquan Smith", Jersey: "0", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3915189.png"},
		DEF2: db.PlayerStatEntry{Name: "Kyle Hamilton", Jersey: "14", Position: "S", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429005.png"},
	},
	"PHI": {
		QB:   db.PlayerStatEntry{Name: "Jalen Hurts", Jersey: "1", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4040715.png"},
		RB:   db.PlayerStatEntry{Name: "Saquon Barkley", Jersey: "26", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3929630.png"},
		WR1:  db.PlayerStatEntry{Name: "A.J. Brown", Jersey: "11", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4047646.png"},
		WR2:  db.PlayerStatEntry{Name: "DeVonta Smith", Jersey: "6", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4241478.png"},
		DEF1: db.PlayerStatEntry{Name: "Zack Baun", Jersey: "53", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3915234.png"},
		DEF2: db.PlayerStatEntry{Name: "Jalen Carter", Jersey: "98", Position: "DT", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4430822.png"},
	},
	"SF": {
		QB:   db.PlayerStatEntry{Name: "Brock Purdy", Jersey: "13", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361741.png"},
		RB:   db.PlayerStatEntry{Name: "Christian McCaffrey", Jersey: "23", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3117251.png"},
		WR1:  db.PlayerStatEntry{Name: "Deebo Samuel", Jersey: "1", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3126486.png"},
		WR2:  db.PlayerStatEntry{Name: "George Kittle", Jersey: "85", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3040151.png"},
		DEF1: db.PlayerStatEntry{Name: "Fred Warner", Jersey: "54", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3124005.png"},
		DEF2: db.PlayerStatEntry{Name: "Nick Bosa", Jersey: "97", Position: "DE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4040605.png"},
	},
	"DET": {
		QB:   db.PlayerStatEntry{Name: "Jared Goff", Jersey: "16", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3046779.png"},
		RB:   db.PlayerStatEntry{Name: "Jahmyr Gibbs", Jersey: "26", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429013.png"},
		WR1:  db.PlayerStatEntry{Name: "Amon-Ra St. Brown", Jersey: "14", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361411.png"},
		WR2:  db.PlayerStatEntry{Name: "Sam LaPorta", Jersey: "87", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4430027.png"},
		DEF1: db.PlayerStatEntry{Name: "Aidan Hutchinson", Jersey: "97", Position: "DE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4362705.png"},
		DEF2: db.PlayerStatEntry{Name: "Alex Anzalone", Jersey: "34", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3052897.png"},
	},
	"BUF": {
		QB:   db.PlayerStatEntry{Name: "Josh Allen", Jersey: "17", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3918298.png"},
		RB:   db.PlayerStatEntry{Name: "James Cook", Jersey: "4", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4362077.png"},
		WR1:  db.PlayerStatEntry{Name: "Khalil Shakir", Jersey: "10", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361418.png"},
		WR2:  db.PlayerStatEntry{Name: "Dalton Kincaid", Jersey: "86", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4372481.png"},
		DEF1: db.PlayerStatEntry{Name: "Terrel Bernard", Jersey: "43", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4240582.png"},
		DEF2: db.PlayerStatEntry{Name: "Greg Rousseau", Jersey: "50", Position: "DE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361309.png"},
	},
	"GB": {
		QB:   db.PlayerStatEntry{Name: "Jordan Love", Jersey: "10", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4036378.png"},
		RB:   db.PlayerStatEntry{Name: "Josh Jacobs", Jersey: "8", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4047365.png"},
		WR1:  db.PlayerStatEntry{Name: "Jayden Reed", Jersey: "11", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361579.png"},
		WR2:  db.PlayerStatEntry{Name: "Romeo Doubs", Jersey: "87", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361546.png"},
		DEF1: db.PlayerStatEntry{Name: "Quay Walker", Jersey: "7", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361408.png"},
		DEF2: db.PlayerStatEntry{Name: "Rashan Gary", Jersey: "52", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4046523.png"},
	},
	"DAL": {
		QB:   db.PlayerStatEntry{Name: "Dak Prescott", Jersey: "4", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/2577417.png"},
		RB:   db.PlayerStatEntry{Name: "Rico Dowdle", Jersey: "23", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4035650.png"},
		WR1:  db.PlayerStatEntry{Name: "CeeDee Lamb", Jersey: "88", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4241389.png"},
		WR2:  db.PlayerStatEntry{Name: "Jake Ferguson", Jersey: "87", Position: "TE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4242488.png"},
		DEF1: db.PlayerStatEntry{Name: "Micah Parsons", Jersey: "11", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361423.png"},
		DEF2: db.PlayerStatEntry{Name: "Trevon Diggs", Jersey: "7", Position: "CB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4040966.png"},
	},
	"HOU": {
		QB:   db.PlayerStatEntry{Name: "C.J. Stroud", Jersey: "7", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4432577.png"},
		RB:   db.PlayerStatEntry{Name: "Joe Mixon", Jersey: "28", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3116385.png"},
		WR1:  db.PlayerStatEntry{Name: "Nico Collins", Jersey: "12", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4258185.png"},
		WR2:  db.PlayerStatEntry{Name: "Stefon Diggs", Jersey: "1", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/2976212.png"},
		DEF1: db.PlayerStatEntry{Name: "Will Anderson Jr.", Jersey: "51", Position: "DE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429020.png"},
		DEF2: db.PlayerStatEntry{Name: "Azeez Al-Shaair", Jersey: "0", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3929814.png"},
	},
	"CIN": {
		QB:   db.PlayerStatEntry{Name: "Joe Burrow", Jersey: "9", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3915511.png"},
		RB:   db.PlayerStatEntry{Name: "Chase Brown", Jersey: "30", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4361536.png"},
		WR1:  db.PlayerStatEntry{Name: "Ja'Marr Chase", Jersey: "1", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4362628.png"},
		WR2:  db.PlayerStatEntry{Name: "Tee Higgins", Jersey: "5", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4239993.png"},
		DEF1: db.PlayerStatEntry{Name: "Trey Hendrickson", Jersey: "91", Position: "DE", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3046467.png"},
		DEF2: db.PlayerStatEntry{Name: "Logan Wilson", Jersey: "55", Position: "LB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3929851.png"},
	},
	"MIA": {
		QB:   db.PlayerStatEntry{Name: "Tua Tagovailoa", Jersey: "1", Position: "QB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4241479.png"},
		RB:   db.PlayerStatEntry{Name: "De'Von Achane", Jersey: "28", Position: "RB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4429018.png"},
		WR1:  db.PlayerStatEntry{Name: "Tyreek Hill", Jersey: "10", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3116406.png"},
		WR2:  db.PlayerStatEntry{Name: "Jaylen Waddle", Jersey: "17", Position: "WR", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/4372016.png"},
		DEF1: db.PlayerStatEntry{Name: "Zach Sieler", Jersey: "92", Position: "DT", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3120894.png"},
		DEF2: db.PlayerStatEntry{Name: "Jalen Ramsey", Jersey: "5", Position: "CB", HeadshotURL: "https://a.espncdn.com/i/headshots/nfl/players/full/3045373.png"},
	},
}

func getRoster(teamCode string, teamName string) defaultRoster {
	if r, ok := nflStarters[teamCode]; ok {
		return r
	}
	// Dynamic fallback
	return defaultRoster{
		QB:   db.PlayerStatEntry{Name: "Titular QB (" + teamCode + ")", Jersey: "7", Position: "QB"},
		RB:   db.PlayerStatEntry{Name: "Corredor Líder (" + teamCode + ")", Jersey: "21", Position: "RB"},
		WR1:  db.PlayerStatEntry{Name: "Receptor Principal (" + teamCode + ")", Jersey: "11", Position: "WR"},
		WR2:  db.PlayerStatEntry{Name: "Ala Cerrada (" + teamCode + ")", Jersey: "88", Position: "TE"},
		DEF1: db.PlayerStatEntry{Name: "Líder Tacleador (" + teamCode + ")", Jersey: "54", Position: "LB"},
		DEF2: db.PlayerStatEntry{Name: "Cazador Mariscal (" + teamCode + ")", Jersey: "99", Position: "DE"},
	}
}

// GenerateRealisticSummary produces a complete, coherent GameDetailedSummary for a finalized game
func GenerateRealisticSummary(game *db.Game) *db.GameDetailedSummary {
	if game == nil {
		return nil
	}

	awayCode := "AWAY"
	awayName := "Visitante"
	awayLogo := ""
	if game.AwayTeam != nil {
		awayCode = game.AwayTeam.Code
		awayName = game.AwayTeam.Name
		awayLogo = game.AwayTeam.LogoURL
	}

	homeCode := "HOME"
	homeName := "Local"
	homeLogo := ""
	if game.HomeTeam != nil {
		homeCode = game.HomeTeam.Code
		homeName = game.HomeTeam.Name
		homeLogo = game.HomeTeam.LogoURL
	}

	awayScore := game.AwayScoreVal()
	homeScore := game.HomeScoreVal()

	// Deterministic pseudo-random seed based on game ID
	seed := int64(game.ID*1000 + 42)
	rnd := rand.New(rand.NewSource(seed))

	// Team stats calculation
	awayStats := buildTeamStats(awayCode, awayName, awayLogo, awayScore, rnd)
	homeStats := buildTeamStats(homeCode, homeName, homeLogo, homeScore, rnd)

	// Balance possession time (~60 mins total)
	awaySecs := 28*60 + rnd.Intn(240)
	homeSecs := 3600 - awaySecs
	awayStats.PossessionTime = fmt.Sprintf("%02d:%02d", awaySecs/60, awaySecs%60)
	homeStats.PossessionTime = fmt.Sprintf("%02d:%02d", homeSecs/60, homeSecs%60)

	// Player stats calculation
	awayRoster := getRoster(awayCode, awayName)
	homeRoster := getRoster(homeCode, homeName)

	awayPlayerStats := buildTeamPlayerStats(awayCode, awayName, awayLogo, awayRoster, awayStats, awayScore, rnd)
	homePlayerStats := buildTeamPlayerStats(homeCode, homeName, homeLogo, homeRoster, homeStats, homeScore, rnd)

	// Scoring plays & offensive drives
	scoringPlays, drives := buildScoringAndDrives(game, awayCode, awayName, awayLogo, homeCode, homeName, homeLogo, awayScore, homeScore, rnd)

	return &db.GameDetailedSummary{
		AwayStats:       awayStats,
		HomeStats:       homeStats,
		AwayPlayerStats: awayPlayerStats,
		HomePlayerStats: homePlayerStats,
		ScoringPlays:    scoringPlays,
		Drives:          drives,
		HasStats:        true,
		HasPlayerStats:  true,
		HasDrives:       len(drives) > 0,
	}
}

func buildTeamStats(code, name, logo string, score int, rnd *rand.Rand) *db.TeamBoxscoreStats {
	// Points dictate realistic yardage
	baseYards := 200 + score*7 + rnd.Intn(35)
	if baseYards < 180 {
		baseYards = 180 + rnd.Intn(40)
	}

	// 60-70% passing, remainder rushing
	passRatio := 0.60 + rnd.Float64()*0.12
	passYards := int(math.Round(float64(baseYards) * passRatio))
	rushYards := baseYards - passYards

	compAttempts := 28 + rnd.Intn(12)
	completions := int(math.Round(float64(compAttempts) * (0.60 + rnd.Float64()*0.15)))
	compAttStr := fmt.Sprintf("%d/%d", completions, compAttempts)

	rushAttempts := 18 + rnd.Intn(10)
	totalPlays := compAttempts + rushAttempts + 4
	yardsPerPlay := fmt.Sprintf("%.1f", float64(baseYards)/float64(totalPlays))

	firstDowns := fmt.Sprintf("%d", 14+score/3+rnd.Intn(5))
	thirdDownsMade := 4 + rnd.Intn(5)
	thirdDownsAtt := 11 + rnd.Intn(4)
	thirdDownEff := fmt.Sprintf("%d-%d", thirdDownsMade, thirdDownsAtt)

	fourthDownsMade := rnd.Intn(2)
	fourthDownsAtt := fourthDownsMade + rnd.Intn(2)
	fourthDownEff := fmt.Sprintf("%d-%d", fourthDownsMade, fourthDownsAtt)

	turnovers := "0"
	if rnd.Float64() < 0.45 {
		turnovers = "1"
	} else if rnd.Float64() < 0.15 {
		turnovers = "2"
	}

	penaltiesCount := 4 + rnd.Intn(5)
	penaltiesYards := penaltiesCount*8 + rnd.Intn(15)
	penalties := fmt.Sprintf("%d-%d", penaltiesCount, penaltiesYards)

	return &db.TeamBoxscoreStats{
		TeamCode:        code,
		TeamName:        name,
		TeamLogoURL:     logo,
		FirstDowns:      firstDowns,
		ThirdDownEff:    thirdDownEff,
		FourthDownEff:   fourthDownEff,
		TotalPlays:      strconv.Itoa(totalPlays),
		TotalYards:      strconv.Itoa(baseYards),
		YardsPerPlay:    yardsPerPlay,
		PassingYards:    strconv.Itoa(passYards),
		CompAtt:         compAttStr,
		RushingYards:    strconv.Itoa(rushYards),
		RushingAttempts: strconv.Itoa(rushAttempts),
		Turnovers:       turnovers,
		Penalties:       penalties,
	}
}

func buildTeamPlayerStats(code, name, logo string, r defaultRoster, tb *db.TeamBoxscoreStats, score int, rnd *rand.Rand) *db.TeamPlayerStats {
	passYds, _ := strconv.Atoi(tb.PassingYards)
	rushYds, _ := strconv.Atoi(tb.RushingYards)

	// Approximate TDs: 1 TD per 7 pts
	totalTDs := score / 7
	passTDs := totalTDs
	rushTDs := 0
	if totalTDs > 1 && rnd.Float64() < 0.5 {
		rushTDs = 1
		passTDs = totalTDs - 1
	}

	// Passing
	parts := strings.Split(tb.CompAtt, "/")
	compStr, attStr := "20", "30"
	if len(parts) == 2 {
		compStr, attStr = parts[0], parts[1]
	}
	intComp, _ := strconv.Atoi(compStr)
	qbr := fmt.Sprintf("%.1f", 85.0+rnd.Float64()*25.0)
	qbEntry := r.QB
	qbEntry.Stats = []string{fmt.Sprintf("%s/%s", compStr, attStr), strconv.Itoa(passYds), strconv.Itoa(passTDs), tb.Turnovers, qbr}

	passingCat := db.PlayerStatCategory{
		Name:    "passing",
		Title:   "Pase",
		Labels:  []string{"C/ATT", "YDS", "TD", "INT", "QBR"},
		Players: []db.PlayerStatEntry{qbEntry},
	}

	// Rushing
	primaryRushYds := int(math.Round(float64(rushYds) * 0.75))
	rushCarries := 14 + rnd.Intn(6)
	avgRush := fmt.Sprintf("%.1f", float64(primaryRushYds)/float64(rushCarries))
	rbEntry := r.RB
	rbEntry.Stats = []string{strconv.Itoa(rushCarries), strconv.Itoa(primaryRushYds), avgRush, strconv.Itoa(rushTDs), fmt.Sprintf("%d", 12+rnd.Intn(18))}

	rushingCat := db.PlayerStatCategory{
		Name:    "rushing",
		Title:   "Acarreo",
		Labels:  []string{"CAR", "YDS", "AVG", "TD", "LARGO"},
		Players: []db.PlayerStatEntry{rbEntry},
	}

	// Receiving
	wr1Yds := int(math.Round(float64(passYds) * 0.55))
	wr1Rec := int(math.Round(float64(intComp) * 0.45))
	if wr1Rec < 3 {
		wr1Rec = 3
	}
	wr2Yds := passYds - wr1Yds
	wr2Rec := intComp - wr1Rec
	if wr2Rec < 2 {
		wr2Rec = 2
	}

	wr1Entry := r.WR1
	wr1Entry.Stats = []string{strconv.Itoa(wr1Rec), strconv.Itoa(wr1Yds), strconv.Itoa(passTDs), fmt.Sprintf("%d", wr1Rec+2), fmt.Sprintf("%d", 22+rnd.Intn(20))}

	wr2Entry := r.WR2
	wr2Entry.Stats = []string{strconv.Itoa(wr2Rec), strconv.Itoa(wr2Yds), "0", fmt.Sprintf("%d", wr2Rec+2), fmt.Sprintf("%d", 14+rnd.Intn(15))}

	receivingCat := db.PlayerStatCategory{
		Name:    "receiving",
		Title:   "Recepción",
		Labels:  []string{"REC", "YDS", "TD", "TAR", "LARGO"},
		Players: []db.PlayerStatEntry{wr1Entry, wr2Entry},
	}

	// Defense
	def1Entry := r.DEF1
	def1Entry.Stats = []string{fmt.Sprintf("%d", 8+rnd.Intn(5)), fmt.Sprintf("%.1f", float64(rnd.Intn(2))), fmt.Sprintf("%d", 1+rnd.Intn(2)), "0"}

	def2Entry := r.DEF2
	def2Entry.Stats = []string{fmt.Sprintf("%d", 4+rnd.Intn(4)), fmt.Sprintf("%.1f", float64(1+rnd.Intn(2))), fmt.Sprintf("%d", 1+rnd.Intn(3)), "0"}

	defensiveCat := db.PlayerStatCategory{
		Name:    "defensive",
		Title:   "Defensiva",
		Labels:  []string{"TAC", "SCK", "TFL", "INT"},
		Players: []db.PlayerStatEntry{def1Entry, def2Entry},
	}

	return &db.TeamPlayerStats{
		TeamCode:   code,
		TeamName:   name,
		TeamLogo:   logo,
		Categories: []db.PlayerStatCategory{passingCat, rushingCat, receivingCat, defensiveCat},
	}
}

func buildScoringAndDrives(
	game *db.Game,
	awayCode, awayName, awayLogo string,
	homeCode, homeName, homeLogo string,
	awayScore, homeScore int,
	rnd *rand.Rand,
) ([]db.ScoringPlayItem, []db.DriveItem) {
	scoring := make([]db.ScoringPlayItem, 0)
	drives := make([]db.DriveItem, 0)

	lines := game.LinescoreData()
	curAway, curHome := 0, 0

	addScore := func(q int, clock, text string, teamCode, teamLogo string, pts int, isAway bool) {
		if isAway {
			curAway += pts
		} else {
			curHome += pts
		}
		scoring = append(scoring, db.ScoringPlayItem{
			Quarter:     q,
			Clock:       clock,
			Text:        text,
			AwayScore:   curAway,
			HomeScore:   curHome,
			TeamCode:    teamCode,
			TeamLogoURL: teamLogo,
		})
	}

	addDrive := func(q int, teamCode, teamName, teamLogo, resCode, resLabel string, plays, yds int, clock, dur string) {
		drives = append(drives, db.DriveItem{
			ID:            fmt.Sprintf("d-%d-%d", q, len(drives)+1),
			TeamCode:      teamCode,
			TeamName:      teamName,
			TeamLogoURL:   teamLogo,
			Description:   fmt.Sprintf("%d jugadas, %d yds, %s", plays, yds, dur),
			PlaysCount:    plays,
			Yards:         yds,
			TimeElapsed:   dur,
			StartPeriod:   q,
			StartClock:    clock,
			StartField:    fmt.Sprintf("%s 25", teamCode),
			EndField:      fmt.Sprintf("%s 0", teamCode),
			Result:        resCode,
			DisplayResult: resLabel,
			IsScore:       resCode == "TD" || resCode == "FG",
			IsCurrent:     false,
		})
	}

	// If quarter breakdown exists in game.Linescores, reconstruct from it!
	if lines != nil && len(lines.AwayScores) > 0 {
		for i := 0; i < len(lines.AwayScores); i++ {
			q := i + 1
			qAway, _ := strconv.Atoi(lines.AwayScores[i])
			qHome, _ := strconv.Atoi(lines.HomeScores[i])

			if qAway == 7 {
				addScore(q, "07:35", fmt.Sprintf("Pase de %d yds para Touchdown", 12+rnd.Intn(25)), awayCode, awayLogo, 7, true)
				addDrive(q, awayCode, awayName, awayLogo, "TD", "Touchdown", 7+rnd.Intn(4), 65+rnd.Intn(20), "11:20", "03:45")
			} else if qAway == 3 {
				addScore(q, "03:12", fmt.Sprintf("Gol de campo de %d yds", 32+rnd.Intn(18)), awayCode, awayLogo, 3, true)
				addDrive(q, awayCode, awayName, awayLogo, "FG", "Gol de Campo", 9+rnd.Intn(3), 48+rnd.Intn(15), "07:15", "04:03")
			} else if qAway > 7 {
				addScore(q, "10:14", fmt.Sprintf("Acarreo de %d yds para Touchdown", 2+rnd.Intn(8)), awayCode, awayLogo, 7, true)
				addDrive(q, awayCode, awayName, awayLogo, "TD", "Touchdown", 8+rnd.Intn(3), 72+rnd.Intn(12), "14:15", "04:01")
				rem := qAway - 7
				if rem >= 3 {
					addScore(q, "01:05", fmt.Sprintf("Gol de campo de %d yds", 30+rnd.Intn(15)), awayCode, awayLogo, rem, true)
					addDrive(q, awayCode, awayName, awayLogo, "FG", "Gol de Campo", 6+rnd.Intn(2), 35+rnd.Intn(15), "03:30", "02:25")
				}
			}

			if qHome == 7 {
				addScore(q, "05:22", fmt.Sprintf("Pase de %d yds para Touchdown", 15+rnd.Intn(20)), homeCode, homeLogo, 7, false)
				addDrive(q, homeCode, homeName, homeLogo, "TD", "Touchdown", 8+rnd.Intn(4), 75+rnd.Intn(10), "09:40", "04:18")
			} else if qHome == 3 {
				addScore(q, "00:45", fmt.Sprintf("Gol de campo de %d yds", 35+rnd.Intn(14)), homeCode, homeLogo, 3, false)
				addDrive(q, homeCode, homeName, homeLogo, "FG", "Gol de Campo", 10+rnd.Intn(3), 52+rnd.Intn(15), "04:50", "04:05")
			} else if qHome > 7 {
				addScore(q, "08:10", fmt.Sprintf("Acarreo de %d yds para Touchdown", 1+rnd.Intn(6)), homeCode, homeLogo, 7, false)
				addDrive(q, homeCode, homeName, homeLogo, "TD", "Touchdown", 9+rnd.Intn(3), 70+rnd.Intn(15), "12:30", "04:20")
				rem := qHome - 7
				if rem >= 3 {
					addScore(q, "00:02", fmt.Sprintf("Gol de campo de %d yds", 28+rnd.Intn(16)), homeCode, homeLogo, rem, false)
					addDrive(q, homeCode, homeName, homeLogo, "FG", "Gol de Campo", 7+rnd.Intn(2), 40+rnd.Intn(12), "02:10", "02:08")
				}
			}
		}
	} else {
		// Generic quarters fallback matching final scores
		if awayScore >= 7 {
			addScore(1, "06:40", "Pase de 14 yds para Touchdown", awayCode, awayLogo, 7, true)
			addDrive(1, awayCode, awayName, awayLogo, "TD", "Touchdown", 8, 70, "10:15", "03:35")
		}
		if homeScore >= 7 {
			addScore(2, "08:15", "Acarreo de 4 yds para Touchdown", homeCode, homeLogo, 7, false)
			addDrive(2, homeCode, homeName, homeLogo, "TD", "Touchdown", 9, 75, "12:40", "04:25")
		}
		if awayScore >= 14 {
			addScore(3, "04:12", "Pase de 22 yds para Touchdown", awayCode, awayLogo, 7, true)
			addDrive(3, awayCode, awayName, awayLogo, "TD", "Touchdown", 6, 68, "07:20", "03:08")
		}
		if homeScore >= 14 {
			addScore(4, "09:30", "Pase de 18 yds para Touchdown", homeCode, homeLogo, 7, false)
			addDrive(4, homeCode, homeName, homeLogo, "TD", "Touchdown", 7, 72, "13:10", "03:40")
		}
		if awayScore%7 != 0 {
			addScore(4, "02:15", "Gol de campo de 37 yds", awayCode, awayLogo, awayScore%7, true)
			addDrive(4, awayCode, awayName, awayLogo, "FG", "Gol de Campo", 8, 45, "05:00", "02:45")
		}
		if homeScore%7 != 0 {
			addScore(4, "00:04", "Gol de campo de 41 yds", homeCode, homeLogo, homeScore%7, false)
			addDrive(4, homeCode, homeName, homeLogo, "FG", "Gol de Campo", 9, 50, "02:15", "02:11")
		}
	}

	return scoring, drives
}
