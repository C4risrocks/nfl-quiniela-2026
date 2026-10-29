package espn

import (
	"encoding/json"
	"fmt"
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

// OfficialWeek3PowerRankings holds the verified ESPN NFL Power Rankings after Week 3 2026
var OfficialWeek3PowerRankings = []*db.TeamPowerRanking{
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "KC",
		Rank:         1,
		PreviousRank: 4,
		RankChange:   3,
		Record:       "3-0",
		Analysis:     "¿Volvieron los Chiefs? Volvieron los Chiefs. Un 3-0 bien ganado, con un Patrick Mahomes (¡y Travis Kelce!) renovado. Ante Las Vegas, tendrán un duelo ante los Raiders significativo por primera vez en temporadas. Aún así, parten como favoritos frente a sus viejos rivales.",
		Author:       "Eric Gómez",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "SF",
		Rank:         2,
		PreviousRank: 3,
		RankChange:   1,
		Record:       "3-0",
		Analysis:     "Tantas lesiones y se las arreglaron para seguir invictos en la campaña. Kyle Shanahan demostró que tiene un libro de opciones y que van a pelear la cima de la NFC Oeste.",
		Author:       "Fernando Villa",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "BUF",
		Rank:         3,
		PreviousRank: 2,
		RankChange:   -1,
		Record:       "3-0",
		Analysis:     "Qué importante es ganar hasta jugando mal. ¿Habrá sido este el peor partido de la temporada regular de Josh Allen y Buffalo?",
		Author:       "Víctor Manuel Macín",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "DEN",
		Rank:         4,
		PreviousRank: 9,
		RankChange:   5,
		Record:       "2-1",
		Analysis:     "Vencieron a los Rams en uno de los juegos nocturnos más entretenidos del año --ojalá y se puedan reencontrar más adelante--. De destacarse el hecho que anotaron sus 30 puntos en prácticamente un cuarto y medio.",
		Author:       "Luis Miguel Vasavilbaso",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "BAL",
		Rank:         5,
		PreviousRank: 12,
		RankChange:   7,
		Record:       "2-1",
		Analysis:     "No hay forma más futbolera de salir del Maracaná que en hombros después de meter el gol (de campo) de la victoria en el último segundo, un día que jamás olvidará Tyler Loop.",
		Author:       "Erick Cervantes Roon",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "CHI",
		Rank:         6,
		PreviousRank: 19,
		RankChange:   13,
		Record:       "2-1",
		Analysis:     "La habilidad y talento de Caleb Williams la potencian, pero la victoria sobre los Eagles con el veterano QB Case Keenum al mando es testimonio de que el sistema ofensivo de los Bears es la clave de su reciente éxito.",
		Author:       "Ivis Aburto",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "MIN",
		Rank:         7,
		PreviousRank: 10,
		RankChange:   3,
		Record:       "3-0",
		Analysis:     "Los próximos cuatro rivales en el calendario son: Dolphins, Saints, Colts y Lions. Así, la probabilidad de que lleguen 7-0 a su duelo contra los Bills no luce taaaaan descabellada.",
		Author:       "Alfonso Mancilla",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "JAX",
		Rank:         8,
		PreviousRank: 17,
		RankChange:   9,
		Record:       "2-1",
		Analysis:     "Auténtico dominio de unos Jaguars que brillaron frente a unos desconocidos Patriots. Trevor Lawrence ha demostrado que puede ser el quarterback y el líder que Jacksonville necesita para trascender en los playoffs. Los Jaguars están encaminados a ganar la AFC Sur.",
		Author:       "Jorge Garrett",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "SEA",
		Rank:         9,
		PreviousRank: 1,
		RankChange:   -8,
		Record:       "2-1",
		Analysis:     "Un descalabro inesperado contra los Commanders es lo necesario para despertar a los campeones antes de su enfrentamiento frente a los Chargers, quienes buscan sumar su primera victoria en una temporada de terror.",
		Author:       "Eric Gómez",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "DET",
		Rank:         10,
		PreviousRank: 16,
		RankChange:   6,
		Record:       "2-1",
		Analysis:     "Tuvieron un partido más complicado de lo previsto, pero Jahmyr Gibbs fue la salvación en el ataque terrestre y gracias a él consiguieron la victoria.",
		Author:       "Fernando Villa",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "LV",
		Rank:         11,
		PreviousRank: 14,
		RankChange:   3,
		Record:       "3-0",
		Analysis:     "Nadie, absolutamente nadie, veía un 3-0 de Raiders. Pero siguen los Chiefs y las apuestas en Las Vegas deben pagar muy bien la victoria local.",
		Author:       "Víctor Manuel Macín",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "PIT",
		Rank:         12,
		PreviousRank: 22,
		RankChange:   10,
		Record:       "2-1",
		Analysis:     "Su victoria permitió que todos los equipos de la AFC Norte ostenten marca de 2-1 y también lavó un poco la imagen que habían dejado una semana atrás en Foxboro.",
		Author:       "Luis Miguel Vasavilbaso",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "LAR",
		Rank:         13,
		PreviousRank: 6,
		RankChange:   -7,
		Record:       "1-2",
		Analysis:     "Con 1-2, los Rams están coqueteando peligrosamente con la etiqueta de mayor decepción de la temporada.",
		Author:       "Erick Cervantes Roon",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "PHI",
		Rank:         14,
		PreviousRank: 7,
		RankChange:   -7,
		Record:       "2-1",
		Analysis:     "El potencial está allí, pero la ejecución y mala toma de decisiones desde la banca no ayudan a alcanzarlo. Jalen Hurts no lo puede hacer todo.",
		Author:       "Ivis Aburto",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "CIN",
		Rank:         15,
		PreviousRank: 5,
		RankChange:   -10,
		Record:       "1-2",
		Analysis:     "La derrota frente a Steelers les dará la dosis de realidad que el equipo necesitaba. Siguen perdiendo los duelos importantes.",
		Author:       "Alfonso Mancilla",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "DAL",
		Rank:         16,
		PreviousRank: 11,
		RankChange:   -5,
		Record:       "1-2",
		Analysis:     "Llevamos tres partidos de la temporada y los Cowboys no encuentran el rumbo. El apuntado debe ser George Pickens, quien tuvo una muy destacada pretemporada, pero que simplemente no encuentra el nivel. Ya son cinco partidos sin anotación para él. Hora de despertar para la ofensiva de Dallas.",
		Author:       "Jorge Garrett",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "NYG",
		Rank:         17,
		PreviousRank: 25,
		RankChange:   8,
		Record:       "1-2",
		Analysis:     "Luego de perder a Jaxson Dart, comenzó el show de Jameis Winston, quien los guió a una victoria. ¿La mala noticia? El rival fueron los Titans, un equipo bastante pobre. Esta semana, la dificultad no subirá mucho ante los Arizona Cardinals.",
		Author:       "Eric Gómez",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "IND",
		Rank:         18,
		PreviousRank: 21,
		RankChange:   3,
		Record:       "1-2",
		Analysis:     "Demostraron que tener un buen pateador es clave y Spencer Shrader tuvo nueve de los 19 puntos del equipo. Mucho por mejorar en el equipo de Shane Steichen, pero al menos ya ganaron.",
		Author:       "Fernando Villa",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "WSH",
		Rank:         19,
		PreviousRank: 28,
		RankChange:   9,
		Record:       "2-1",
		Analysis:     "Sam Darnold completó 31 de 45 pases para 379 yardas y 4 TD. No es que los Seahawks jugaran mal, pero la ofensiva encabezada por Marcus Mariota no paró de contestar los puntos rivales y la defensiva se encargó de interceptar dos veces al QB campeón de la NFL; una de esas jugadas grandes selló la sorpresiva victoria.",
		Author:       "Víctor Manuel Macín",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "CLE",
		Rank:         20,
		PreviousRank: 20,
		RankChange:   0,
		Record:       "1-2",
		Analysis:     "Su debut en casa estuvo acompañado de una victoria importante ante los Panthers detrás de una defensiva de respeto, pero ahora veremos si mantienen el paso cuando abran su calendario divisional ante los Steelers.",
		Author:       "Luis Miguel Vasavilbaso",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "ATL",
		Rank:         21,
		PreviousRank: 32,
		RankChange:   11,
		Record:       "1-2",
		Analysis:     "Los Falcons llegaron como víctimas a Lambeau Field, pero Bijan Robinson tenía otros planes y demostró por qué es el mejor corredor de la NFL en la actualidad.",
		Author:       "Erick Cervantes Roon",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "NE",
		Rank:         22,
		PreviousRank: 8,
		RankChange:   -14,
		Record:       "1-2",
		Analysis:     "Drake Maye se ve como un QB con clara desconfianza en sí mismo y que necesita un abrazo de su coach y coordinador ofensivo que le ayude a recuperar confianza.",
		Author:       "Ivis Aburto",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "CAR",
		Rank:         23,
		PreviousRank: 15,
		RankChange:   -8,
		Record:       "1-2",
		Analysis:     "La buena, siguen como líderes divisionales. La mala: lideran la peor división de la NFL. La peor: los equipos comenzarán a castigar más duro a Bryce Young.",
		Author:       "Alfonso Mancilla",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "NO",
		Rank:         24,
		PreviousRank: 18,
		RankChange:   -6,
		Record:       "1-2",
		Analysis:     "No sé si alguien tenía grandes expectativas de los Saints antes de la temporada. Al menos yo no. En todo caso, han sido un equipo batallador. Casi dan la sorpresa frente a Lions, vencieron a Ravens y se quedaron cerca del triunfo ante Raiders. Pulgar arriba para los Saints en estas primeras semanas.",
		Author:       "Jorge Garrett",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "GB",
		Rank:         25,
		PreviousRank: 13,
		RankChange:   -12,
		Record:       "1-2",
		Analysis:     "De no ser por los Jets, Green Bay estaría 0-3, algo impensable al inicio de la temporada. Hay algo seriamente mal con la defensiva del equipo, sin Micah Parsons, y la ofensiva tampoco ha sido de lo más brillante. Los Buccaneers, rival de la Semana 4, están aún peor. ¿Quién sale avante?.",
		Author:       "Eric Gómez",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "HOU",
		Rank:         26,
		PreviousRank: 23,
		RankChange:   -3,
		Record:       "0-3",
		Analysis:     "DeMeco Ryans no encuentra las respuestas para darle a C.J. Stroud una mejor ofensiva y más opciones para anotar. No parece que vayan a levantar pronto el paso.",
		Author:       "Fernando Villa",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "NYJ",
		Rank:         27,
		PreviousRank: 26,
		RankChange:   -1,
		Record:       "1-2",
		Analysis:     "Buen juego de New York en Detroit: Geno Smith, 31/37, 321 yardas, 3TD ¡0 intercepciones! El cuerpo de receptores dio un partidazo con grandes recepciones, pero la defensiva no pudo contener a Goff y Gibbs y ahí perdieron el juego. Pero si Geno entrega consistentemente actuaciones como esta, los Jets van a superar expectativas.",
		Author:       "Víctor Manuel Macín",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "ARI",
		Rank:         28,
		PreviousRank: 27,
		RankChange:   -1,
		Record:       "1-2",
		Analysis:     "La defensiva volvió a permitir más de 30 puntos y eso muy pocas veces se traducirá en triunfos, pero hay que rescatar la reacción que tuvieron ante los 49ers, contrario a lo que ocurrió una semana atrás contra los Seahawks.",
		Author:       "Luis Miguel Vasavilbaso",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "TB",
		Rank:         29,
		PreviousRank: 24,
		RankChange:   -5,
		Record:       "1-2",
		Analysis:     "Su temporada ya iba camino al abismo con Baker Mayfield sano, ahora que tendrán que jugar sin él las próximas tres semanas no se sorprendan si los vemos convertidos en un equipo 0-6.",
		Author:       "Erick Cervantes Roon",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "LAC",
		Rank:         30,
		PreviousRank: 30,
		RankChange:   0,
		Record:       "0-3",
		Analysis:     "Con récord de 0-3, los Chargers sí compiten por algo, con los Buccaneers por la etiqueta de la mayor decepción de la temporada 2026.",
		Author:       "Ivis Aburto",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "TEN",
		Rank:         31,
		PreviousRank: 29,
		RankChange:   -2,
		Record:       "0-3",
		Analysis:     "Es uno de los equipos que luce como sólido contendiente… a obtener el pick N° 1 Draft 2027.",
		Author:       "Alfonso Mancilla",
		UpdatedAt:    time.Now(),
	},
	{
		SeasonYear:   2026,
		WeekNumber:   3,
		TeamCode:     "MIA",
		Rank:         32,
		PreviousRank: 31,
		RankChange:   -1,
		Record:       "0-3",
		Analysis:     "Los Dolphins simplemente no saben nadar. Son por mucho el peor equipo de la NFL; no se les ve un norte. Sí, perdieron contra uno de los tres mejores equipos en la Semana 3, pero no hay un rumbo prometedor para Miami. Será un largo año para los Fins.",
		Author:       "Jorge Garrett",
		UpdatedAt:    time.Now(),
	},
}

// Known2026PowerRankingsURLs holds verified direct URLs to official weekly ESPN NFL Power Rankings articles
var Known2026PowerRankingsURLs = map[int]string{
	1: "https://www.espn.com/nfl/story/_/id/49754534/2026-nfl-power-rankings-preseason-milestones",
	2: "https://www.espn.com/nfl/story/_/id/49881711/power-rankings-week-2-all-teams-top-newcomers-debuts",
	3: "https://www.espn.com/nfl/story/_/id/49998616/nfl-power-rankings-week-3-teams-quarterbacks-stats",
	4: "https://www.espn.com/nfl/story/_/id/50052289/nfl-power-rankings-week-4-teams-fantasy-surprises",
	5: "https://www.espn.com/nfl/story/_/id/50109845/nfl-power-rankings-week-5-teams-first-month-recap",
	6: "https://www.espn.com/nfl/story/_/id/50166454/nfl-power-rankings-week-6-teams-biggest-issue-defense",
	7: "https://www.espn.com/nfl/story/_/id/50218886/nfl-power-rankings-week-7-teams-biggest-issue-offense",
	8: "https://www.espn.com/nfl/story/_/id/50288293/nfl-power-rankings-week-8-teams-best-offseason-additions",
}

// GetLatestKnownPowerRankingsWeek returns the highest week number available in Known2026PowerRankingsURLs
func GetLatestKnownPowerRankingsWeek(seasonYear int) int {
	if seasonYear == 2026 {
		maxW := 1
		for w := range Known2026PowerRankingsURLs {
			if w > maxW {
				maxW = w
			}
		}
		return maxW
	}
	return 1
}

var (
	rankHeaderRegex = regexp.MustCompile(`(?s)<h2[^>]*>\s*([0-9]{1,2})\.\s*<a[^>]*href=["'](?:https?://(?:www\.)?espn\.com)?/nfl/team/_/name/([^/'"]+)/[^"']*["'][^>]*>([^<]+)</a>(?:\s*\(([0-9]+-[0-9]+(?:-[0-9]+)?)\))?</h2>`)
	prevRankRegex   = regexp.MustCompile(`(?i)(?:Week\s*[0-9]+\s*ranking|Previous\s*ranking|Postdraft\s*ranking):?\s*</strong>?\s*([0-9]{1,2})`)
	authorRegex     = regexp.MustCompile(`(?:<em>\s*)?(?:--|—|&mdash;)\s*([A-ZÁÉÍÓÚÑ][A-Za-zÁÉÍÓÚáéíóúñ\s.']+?)(?:,\s*NFL Nation)?(?:</em>|$)`)
	htmlTagRegex    = regexp.MustCompile(`<[^>]+>`)
	spaceRegex      = regexp.MustCompile(`\s+`)
	storyIDRegex    = regexp.MustCompile(`/id/([0-9]+)`)

	articleRankingsCache   = make(map[string][]*db.TeamPowerRanking)
	articleRankingsCacheMu sync.RWMutex
	articleCacheTTL        = 1 * time.Hour
	articleCacheTime       = make(map[string]time.Time)
)

func InvalidateArticleRankingsCache() {
	articleRankingsCacheMu.Lock()
	defer articleRankingsCacheMu.Unlock()
	articleRankingsCache = make(map[string][]*db.TeamPowerRanking)
	articleCacheTime = make(map[string]time.Time)
}

// FindPowerRankingsArticleURL discovers the official ESPN Power Rankings article URL for a given week
func (c *Client) FindPowerRankingsArticleURL(seasonYear, weekNumber int) (string, error) {
	if seasonYear == 2026 {
		if u, ok := Known2026PowerRankingsURLs[weekNumber]; ok {
			return u, nil
		}
	}

	searchQueries := []string{
		fmt.Sprintf("NFL+Power+Rankings+%d+week+%d", seasonYear, weekNumber),
		fmt.Sprintf("NFL+Power+Rankings+week+%d", weekNumber),
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	weekPattern := regexp.MustCompile(fmt.Sprintf(`(?i)\bweek[- ]%d\b`, weekNumber))

	type searchContent struct {
		DisplayName string `json:"displayName"`
		Link        struct {
			Web string `json:"web"`
		} `json:"link"`
	}
	type searchItem struct {
		Contents []searchContent `json:"contents"`
	}
	type searchResp struct {
		Results []searchItem `json:"results"`
	}

	for _, searchQuery := range searchQueries {
		searchURL := fmt.Sprintf("https://site.web.api.espn.com/apis/search/v2?query=%s&limit=15", searchQuery)

		req, err := http.NewRequest("GET", searchURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")

		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			continue
		}

		var parsed searchResp
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		for _, item := range parsed.Results {
			for _, content := range item.Contents {
				link := content.Link.Web
				if strings.Contains(link, "nfl/story") && strings.Contains(link, "power-rankings") {
					if weekPattern.MatchString(link) || weekPattern.MatchString(content.DisplayName) {
						return link, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("no ESPN Power Rankings article found for season %d week %d", seasonYear, weekNumber)
}

// ParseESPNPowerRankingsHTML extracts all 32 ranked franchises from an ESPN article HTML page
func ParseESPNPowerRankingsHTML(htmlContent string, seasonYear, weekNumber int, teamMap map[string]*db.Team) ([]*db.TeamPowerRanking, error) {
	indices := rankHeaderRegex.FindAllStringSubmatchIndex(htmlContent, -1)
	if len(indices) < 20 {
		return nil, fmt.Errorf("insufficient team rankings found in article (found %d)", len(indices))
	}

	now := time.Now()
	rankings := make([]*db.TeamPowerRanking, 0, len(indices))

	for i, idx := range indices {
		rankStr := htmlContent[idx[2]:idx[3]]
		teamCodeRaw := htmlContent[idx[4]:idx[5]]
		teamCode := NormalizeTeamCode(strings.ToUpper(teamCodeRaw))
		teamName := strings.TrimSpace(htmlContent[idx[6]:idx[7]])
		record := ""
		if idx[8] != -1 && idx[9] != -1 {
			record = htmlContent[idx[8]:idx[9]]
		}

		rank, _ := strconv.Atoi(rankStr)

		// Determine body chunk between this team and next
		blockStart := idx[1]
		blockEnd := len(htmlContent)
		if i+1 < len(indices) {
			blockEnd = indices[i+1][0]
		}
		bodyChunk := htmlContent[blockStart:blockEnd]

		// Extract previous week ranking
		prevRank := rank
		if pm := prevRankRegex.FindStringSubmatch(bodyChunk); len(pm) > 1 {
			if pr, err := strconv.Atoi(pm[1]); err == nil && pr > 0 {
				prevRank = pr
			}
		}

		// Extract author
		author := "ESPN NFL Nation"
		if am := authorRegex.FindStringSubmatch(bodyChunk); len(am) > 1 {
			author = strings.TrimSpace(am[1])
		}

		// Clean body text for analysis
		clean := htmlTagRegex.ReplaceAllString(bodyChunk, " ")
		clean = spaceRegex.ReplaceAllString(clean, " ")
		clean = strings.TrimSpace(clean)

		if cutIdx := strings.Index(clean, "Editor's Picks"); cutIdx != -1 {
			clean = strings.TrimSpace(clean[:cutIdx])
		}
		if cutIdx := strings.Index(clean, "-- "+author); cutIdx != -1 {
			clean = strings.TrimSpace(clean[:cutIdx])
		}

		pr := &db.TeamPowerRanking{
			SeasonYear:   seasonYear,
			WeekNumber:   weekNumber,
			TeamCode:     teamCode,
			TeamName:     teamName,
			Rank:         rank,
			PreviousRank: prevRank,
			RankChange:   prevRank - rank,
			Record:       record,
			Analysis:     clean,
			Author:       author,
			UpdatedAt:    now,
		}

		// Enrich with metadata from teamMap
		if t, ok := teamMap[teamCode]; ok {
			pr.TeamID = t.ID
			if t.Name != "" {
				pr.TeamName = t.Name
			}
			pr.TeamCity = t.City
			pr.LogoURL = t.LogoURL
			pr.PrimaryColor = t.PrimaryColor
			pr.Conference = t.Conference
			pr.Division = t.Division
		}

		rankings = append(rankings, pr)
	}

	return rankings, nil
}

// FetchOfficialArticlePowerRankings fetches and parses the official ESPN editorial Power Rankings for a given week
func (c *Client) FetchOfficialArticlePowerRankings(seasonYear, weekNumber int, teamMap map[string]*db.Team) ([]*db.TeamPowerRanking, error) {
	cacheKey := fmt.Sprintf("%d-%d", seasonYear, weekNumber)
	articleRankingsCacheMu.RLock()
	cached, found := articleRankingsCache[cacheKey]
	cacheTime := articleCacheTime[cacheKey]
	articleRankingsCacheMu.RUnlock()

	if found && len(cached) > 0 && time.Since(cacheTime) < articleCacheTTL {
		result := make([]*db.TeamPowerRanking, len(cached))
		for i, orig := range cached {
			cp := *orig
			if t, ok := teamMap[cp.TeamCode]; ok {
				cp.TeamID = t.ID
				if t.Name != "" {
					cp.TeamName = t.Name
				}
				cp.TeamCity = t.City
				cp.LogoURL = t.LogoURL
				cp.PrimaryColor = t.PrimaryColor
				cp.Conference = t.Conference
				cp.Division = t.Division
			}
			result[i] = &cp
		}
		return result, nil
	}

	articleURL, err := c.FindPowerRankingsArticleURL(seasonYear, weekNumber)
	if err != nil {
		return nil, err
	}

	httpClient := c.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	var htmlContent string

	// Strategy 1: Fast CDN JSON API (immune to Akamai/CloudFront blocks on cloud hosts)
	if idMatch := storyIDRegex.FindStringSubmatch(articleURL); len(idMatch) > 1 {
		cdnURL := fmt.Sprintf("https://cdn.espn.com/core/nfl/story?xhr=1&id=%s", idMatch[1])
		if cdnReq, err := http.NewRequest("GET", cdnURL, nil); err == nil {
			cdnReq.Header.Set("User-Agent", "curl/8.7.1")
			cdnReq.Header.Set("Accept", "*/*")
			if cdnResp, err := httpClient.Do(cdnReq); err == nil {
				if cdnResp.StatusCode >= 200 && cdnResp.StatusCode < 300 {
					var payload struct {
						Content struct {
							Story string `json:"story"`
						} `json:"content"`
					}
					if err := json.NewDecoder(cdnResp.Body).Decode(&payload); err == nil && len(payload.Content.Story) > 1000 {
						htmlContent = payload.Content.Story
					}
				}
				cdnResp.Body.Close()
			}
		}
	}

	// Strategy 2: Direct web article request if CDN is unavailable or empty
	if htmlContent == "" {
		req, err := http.NewRequest("GET", articleURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("article request failed with status %d", resp.StatusCode)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		htmlContent = string(bodyBytes)
	}

	rankings, err := ParseESPNPowerRankingsHTML(htmlContent, seasonYear, weekNumber, teamMap)
	if err != nil {
		return nil, err
	}

	// Cache successful result
	articleRankingsCacheMu.Lock()
	articleRankingsCache[cacheKey] = rankings
	articleCacheTime[cacheKey] = time.Now()
	articleRankingsCacheMu.Unlock()

	return rankings, nil
}

// FetchPowerRankings returns the weekly ESPN Power Rankings, querying the official ESPN editorial article first,
// with dynamic algorithmic calculation and 100% offline fallback when article is unreachable.
func (c *Client) FetchPowerRankings(seasonYear, weekNumber int, teamMap map[string]*db.Team) ([]*db.TeamPowerRanking, error) {
	if weekNumber <= 0 {
		weekNumber = 3
	}

	// 1. First attempt to fetch the official ESPN editorial article dynamically
	articleRankings, err := c.FetchOfficialArticlePowerRankings(seasonYear, weekNumber, teamMap)
	if err == nil && len(articleRankings) == 32 {
		return articleRankings, nil
	}

	// 2. If article fetch failed or was offline, check if verified editorial baseline exists
	if seasonYear == 2026 {
		if verified, ok := Verified2026EditorialPowerRankings[weekNumber]; ok && len(verified) == 32 {
			result := make([]*db.TeamPowerRanking, len(verified))
			for i, orig := range verified {
				cp := *orig
				if t, ok := teamMap[cp.TeamCode]; ok {
					cp.TeamID = t.ID
					cp.TeamName = t.Name
					cp.TeamCity = t.City
					cp.LogoURL = t.LogoURL
					cp.PrimaryColor = t.PrimaryColor
					cp.Conference = t.Conference
					cp.Division = t.Division
				}
				result[i] = &cp
			}
			return result, nil
		}
	}

	// 3. Fallback: generate algorithmic rankings for future unverified weeks when ESPN is offline
	standings, _ := c.FetchNFLStandings(seasonYear, seasonYear, teamMap)
	prevWeekRanks := make(map[string]int)
	for _, pr := range OfficialWeek3PowerRankings {
		prevWeekRanks[pr.TeamCode] = pr.Rank
	}
	return CalculateAlgorithmicPowerRankings(seasonYear, weekNumber, standings, prevWeekRanks, teamMap), nil
}

// CalculateAlgorithmicPowerRankings generates weekly Power Rankings for any season week
// based on win percentage, point differential, Pythagorean expectation, and streaks.
func CalculateAlgorithmicPowerRankings(
	seasonYear, weekNumber int,
	standings *db.SeasonStandings,
	prevRankings map[string]int,
	teamMap map[string]*db.Team,
) []*db.TeamPowerRanking {
	if standings == nil || len(standings.League) == 0 {
		// Fallback to baseline week 3 if no standings available
		rankings := make([]*db.TeamPowerRanking, 0, len(OfficialWeek3PowerRankings))
		for _, orig := range OfficialWeek3PowerRankings {
			pr := *orig
			pr.SeasonYear = seasonYear
			pr.WeekNumber = weekNumber
			if t, ok := teamMap[pr.TeamCode]; ok {
				pr.TeamID = t.ID
				pr.TeamName = t.Name
				pr.TeamCity = t.City
				pr.LogoURL = t.LogoURL
				pr.PrimaryColor = t.PrimaryColor
				pr.Conference = t.Conference
				pr.Division = t.Division
			}
			rankings = append(rankings, &pr)
		}
		return rankings
	}

	type teamEval struct {
		standing   *db.TeamStanding
		powerScore float64
	}

	evals := make([]teamEval, 0, len(standings.League))

	for _, ts := range standings.League {
		score := 0.0

		// 1. Win percentage (0 to 45 pts)
		score += ts.WinPercent * 45.0

		// 2. Normalized point differential (up to +/- 20 pts)
		if ts.GamesPlayed > 0 {
			diffPerGame := float64(ts.PointDiff) / float64(ts.GamesPlayed)
			score += math.Max(math.Min(diffPerGame*1.5, 20.0), -20.0)
		}

		// 3. Pythagorean wins expectation (up to 25 pts)
		if ts.GamesPlayed > 0 {
			score += (ts.PythagoreanWins / float64(ts.GamesPlayed)) * 25.0
		}

		// 4. Streak momentum (+/- 10 pts)
		if strings.HasPrefix(ts.Streak, "W") {
			streakNum, _ := strconv.Atoi(strings.TrimPrefix(ts.Streak, "W"))
			score += math.Min(float64(streakNum)*2.0, 10.0)
		} else if strings.HasPrefix(ts.Streak, "L") {
			streakNum, _ := strconv.Atoi(strings.TrimPrefix(ts.Streak, "L"))
			score -= math.Min(float64(streakNum)*2.0, 10.0)
		}

		evals = append(evals, teamEval{
			standing:   ts,
			powerScore: score,
		})
	}

	// Sort descending by power score
	sort.Slice(evals, func(i, j int) bool {
		return evals[i].powerScore > evals[j].powerScore
	})

	rankings := make([]*db.TeamPowerRanking, 0, len(evals))
	for idx, item := range evals {
		ts := item.standing
		rank := idx + 1

		prevRank := rank
		if pRank, ok := prevRankings[ts.TeamCode]; ok && pRank > 0 {
			prevRank = pRank
		} else if ts.ConferenceSeed > 0 {
			prevRank = ts.ConferenceSeed * 2
		}
		rankChange := prevRank - rank

		rec := fmt.Sprintf("%d-%d", ts.Wins, ts.Losses)
		if ts.Ties > 0 {
			rec = fmt.Sprintf("%d-%d-%d", ts.Wins, ts.Losses, ts.Ties)
		}

		var analysis string
		if rank <= 5 {
			analysis = fmt.Sprintf("Contendiente de primer nivel. Récord de %s con un diferencial de %+d puntos y racha %s. Eficiencia neta de élite (%.1f PPG ofensivos vs %.1f defensivos).",
				rec, ts.PointDiff, ts.Streak, ts.OffensivePPG, ts.DefensivePPG)
		} else if rank <= 14 {
			analysis = fmt.Sprintf("En zona sólida de postemporada. Récord de %s con diferencial de %+d puntos. Promedio anotador de %.1f puntos por partido.",
				rec, ts.PointDiff, ts.OffensivePPG)
		} else if rank <= 24 {
			analysis = fmt.Sprintf("En la pelea en el corte de mitad de tabla. Récord de %s y diferencial de %+d puntos con balance de juego de una sola posesión (%s).",
				rec, ts.PointDiff, ts.OneScoreRecord)
		} else {
			analysis = fmt.Sprintf("Buscando corregir el rumbo en la segunda mitad del calendario. Récord de %s con diferencial de %+d puntos y %.1f PPG permitidos a los rivales.",
				rec, ts.PointDiff, ts.DefensivePPG)
		}

		pr := &db.TeamPowerRanking{
			SeasonYear:   seasonYear,
			WeekNumber:   weekNumber,
			TeamID:       ts.TeamID,
			TeamCode:     ts.TeamCode,
			TeamName:     ts.TeamName,
			TeamCity:     ts.TeamCity,
			LogoURL:      ts.LogoURL,
			PrimaryColor: ts.PrimaryColor,
			Conference:   ts.Conference,
			Division:     ts.Division,
			Rank:         rank,
			PreviousRank: prevRank,
			RankChange:   rankChange,
			Record:       rec,
			Analysis:     analysis,
			Author:       "Modelo Analítico ESPN/Quiniela",
			UpdatedAt:    time.Now(),
		}
		if t, ok := teamMap[ts.TeamCode]; ok && pr.TeamID == 0 {
			pr.TeamID = t.ID
		}

		rankings = append(rankings, pr)
	}

	return rankings
}
