package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"
)

// Spanish day and month translations
var (
	spanishDays = map[string]string{
		"Mon": "Lun", "Tue": "Mar", "Wed": "Mié", "Thu": "Jue", "Fri": "Vie", "Sat": "Sáb", "Sun": "Dom",
	}
	spanishMonths = map[string]string{
		"Jan": "Ene", "Feb": "Feb", "Mar": "Mar", "Apr": "Abr", "May": "May", "Jun": "Jun",
		"Jul": "Jul", "Aug": "Ago", "Sep": "Sep", "Oct": "Oct", "Nov": "Nov", "Dec": "Dic",
	}
	CDMXLocation *time.Location
)

func init() {
	loc, err := time.LoadLocation("America/Mexico_City")
	if err == nil {
		CDMXLocation = loc
	} else {
		CDMXLocation = time.FixedZone("CST", -6*3600)
	}
}

type Renderer struct {
	templatesFS fs.FS
}

func NewRenderer(templatesFS fs.FS) *Renderer {
	return &Renderer{
		templatesFS: templatesFS,
	}
}

func (r *Renderer) FuncMap() template.FuncMap {
	return template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call: odd number of arguments")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"slice": func(s string, start, end int) string {
			if len(s) == 0 {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			if start > end {
				return ""
			}
			return s[start:end]
		},
		"formatDate": func(t time.Time) string {
			if t.IsZero() {
				return "--"
			}
			if CDMXLocation != nil {
				t = t.In(CDMXLocation)
			}
			// Localize to standard display format e.g. "Dom, 8 Sep - 1:00 PM"
			dayAbbr := spanishDays[t.Format("Mon")]
			monthAbbr := spanishMonths[t.Format("Jan")]
			dayNum := t.Format("2")
			hourMin := t.Format("3:04 PM")
			return fmt.Sprintf("%s, %s %s - %s", dayAbbr, dayNum, monthAbbr, hourMin)
		},
		"formatShortDate": func(t time.Time) string {
			if t.IsZero() {
				return "--"
			}
			if CDMXLocation != nil {
				t = t.In(CDMXLocation)
			}
			return t.Format("02/01/2006 15:04")
		},
	}
}

// RenderPage parses base layout + page template and executes it
func (r *Renderer) RenderPage(w http.ResponseWriter, pageFile string, data interface{}) {
	patterns := []string{
		"layouts/base.html",
		"pages/" + pageFile,
		"partials/*.html",
	}

	tmpl, err := template.New("base.html").Funcs(r.FuncMap()).ParseFS(r.templatesFS, patterns...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template parse error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Template execution error: %v", err), http.StatusInternalServerError)
	}
}

// RenderPartial parses and executes an isolated partial without layout
func (r *Renderer) RenderPartial(w http.ResponseWriter, partialFile string, data interface{}) {
	pattern := "partials/" + partialFile
	tmpl, err := template.New(partialFile).Funcs(r.FuncMap()).ParseFS(r.templatesFS, pattern)
	if err != nil {
		http.Error(w, fmt.Sprintf("Partial template parse error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Partial execution error: %v", err), http.StatusInternalServerError)
	}
}
