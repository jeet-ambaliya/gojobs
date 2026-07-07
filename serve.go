package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"
)

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	jobsPath := fs.String("jobs", "jobs.json", "path to the jobs file")
	addr := fs.String("addr", ":8080", "address to listen on")
	maxAgeDays := fs.Int("max-age-days", 60, "hide postings older than this many days (0 = show all)")
	_ = fs.Parse(args)

	tmpl := template.Must(template.New("page").Funcs(template.FuncMap{
		"since":  humanSince,
		"date":   func(t time.Time) string { return t.Format("2006-01-02") },
		"unix":   func(t time.Time) int64 { return t.Unix() },
		"region": regionOf,
	}).Parse(pageTemplate))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		jobs := loadJobsQuiet(*jobsPath) // reload each request so re-scrapes show up live

		// Enforce the freshness window on every request, relative to now — so a
		// posting that ages past the limit disappears without a re-scrape, and
		// stale entries in jobs.json are never shown.
		if *maxAgeDays > 0 {
			cutoff := time.Now().UTC().AddDate(0, 0, -*maxAgeDays)
			fresh := jobs[:0]
			for _, j := range jobs {
				if !j.PostedAt.Before(cutoff) {
					fresh = append(fresh, j)
				}
			}
			jobs = fresh
		}

		data := struct {
			Jobs      []Job
			Count     int
			Generated string
		}{jobs, len(jobs), time.Now().Format("2006-01-02 15:04")}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("render: %v", err)
		}
	})

	log.Printf("Serving %s at http://localhost%s  (Ctrl+C to stop)", *jobsPath, *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

// regionCities are the major Canadian metros we bucket postings into, checked
// in order. The first substring match wins.
var regionCities = []string{
	"Toronto", "Vancouver", "Ottawa", "Calgary", "Edmonton", "Waterloo",
	"Kitchener", "Winnipeg", "Halifax", "Victoria", "Hamilton", "Mississauga",
	"Burnaby",
}

// regionOf collapses a free-text location into a canonical filter bucket: a
// major Canadian city, "Montréal"/"Québec", "Remote", "Canada" (named but no
// city), or "Other" (e.g. a US location kept because the description named
// Canada). Cities win over "Remote" so "Remote — Toronto" buckets as Toronto.
func regionOf(loc string) string {
	l := strings.ToLower(loc)
	for _, c := range regionCities {
		if strings.Contains(l, strings.ToLower(c)) {
			return c
		}
	}
	if strings.Contains(l, "montr") { // Montreal / Montréal
		return "Montréal"
	}
	if strings.Contains(l, "quebec") || strings.Contains(l, "québec") {
		return "Québec"
	}
	// Canada wins over Remote — a "Canada (Remote)" posting is Canadian first;
	// remoteness is covered separately by the "remote only" toggle.
	if strings.Contains(l, "canada") || strings.Contains(l, "canadian") {
		return "Canada"
	}
	if strings.Contains(l, "remote") {
		return "Remote"
	}
	return "Other"
}

func humanSince(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "1d"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
}

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>gojobs · Go roles in Canada</title>
<style>
  :root{
    --bg:#0b1016; --surface:#121924; --surface-2:#0f1620;
    --line:#1e2a38; --text:#e7eef5; --muted:#7d8ea0;
    --go:#00add8; --go-dim:#0b3a48; --maple:#e8564b;
    --mono:ui-monospace,"JetBrains Mono","SF Mono",Menlo,Consolas,monospace;
    --sans:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
  }
  *{box-sizing:border-box}
  body{margin:0;background:var(--bg);color:var(--text);font-family:var(--sans);
    line-height:1.5;-webkit-font-smoothing:antialiased}
  .wrap{max-width:920px;margin:0 auto;padding:32px 20px 80px}
  header{border-bottom:1px solid var(--line);padding-bottom:20px;margin-bottom:24px}
  .cmd{font-family:var(--mono);font-size:13px;color:var(--muted)}
  .cmd .dollar{color:var(--go)}
  .cmd .flag{color:var(--text)}
  h1{font-family:var(--mono);font-weight:700;font-size:30px;letter-spacing:-.5px;
    margin:10px 0 4px}
  h1 .dot{color:var(--go)}
  .tagline{color:var(--muted);font-size:14px;margin:0}
  .controls{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin:22px 0 8px}
  #q{flex:1;min-width:220px;background:var(--surface);border:1px solid var(--line);
    color:var(--text);font-family:var(--mono);font-size:14px;padding:10px 12px;border-radius:8px}
  #q:focus{outline:none;border-color:var(--go)}
  .toggle{font-family:var(--mono);font-size:13px;color:var(--muted);
    display:flex;align-items:center;gap:7px;cursor:pointer;user-select:none;
    background:var(--surface);border:1px solid var(--line);padding:9px 12px;border-radius:8px}
  .toggle input{accent-color:var(--go)}
  .sel{font-family:var(--mono);font-size:13px;color:var(--text);cursor:pointer;
    background:var(--surface);border:1px solid var(--line);padding:9px 12px;border-radius:8px}
  .sel:focus{outline:none;border-color:var(--go)}
  .sel label,.sellabel{color:var(--muted)}
  .field{display:flex;align-items:center;gap:7px;font-family:var(--mono);
    font-size:13px;color:var(--muted)}
  .meta{font-family:var(--mono);font-size:12px;color:var(--muted);margin:4px 2px 20px}
  #shown{color:var(--go)}
  ul{list-style:none;margin:0;padding:0}
  li.job{border:1px solid var(--line);background:var(--surface);border-radius:10px;
    padding:16px 18px;margin-bottom:10px;transition:border-color .12s}
  li.job:hover{border-color:var(--go-dim)}
  .row1{display:flex;justify-content:space-between;gap:14px;align-items:baseline}
  .title{font-weight:600;font-size:16px;text-decoration:none;color:var(--text)}
  .title:hover{color:var(--go)}
  .co{display:block;font-family:var(--mono);font-size:14px;font-weight:600;
    color:var(--go);margin-top:3px;letter-spacing:-.2px}
  .tags{display:flex;gap:6px;flex-wrap:wrap;margin-top:9px;
    font-family:var(--mono);font-size:11px}
  .tag{padding:2px 8px;border-radius:20px;border:1px solid var(--line);color:var(--muted)}
  .tag.loc{border-color:var(--go-dim);color:#8fd6e8}
  .tag.remote{border-color:#5a2b28;color:var(--maple)}
  .tag.exp{border-color:#3a3320;color:#d9c58c}
  .exp-field input{width:56px;background:var(--surface);border:1px solid var(--line);
    color:var(--text);font-family:var(--mono);font-size:13px;padding:8px 6px;border-radius:8px;text-align:center}
  .exp-field input:focus{outline:none;border-color:var(--go)}
  .exp-field .dash{color:var(--muted)}
  .tag.age{margin-left:auto;border:none;color:var(--muted)}
  .snip{color:var(--muted);font-size:13px;margin-top:10px}
  .empty{font-family:var(--mono);color:var(--muted);border:1px dashed var(--line);
    border-radius:10px;padding:40px 20px;text-align:center}
  footer{margin-top:30px;font-family:var(--mono);font-size:11px;color:var(--muted)}
  @media (prefers-reduced-motion:reduce){*{transition:none!important}}
</style>
</head>
<body>
<div class="wrap">
  <header>
    <div class="cmd"><span class="dollar">$</span> gojobs serve <span class="flag">--lang=go --country=CA</span></div>
    <h1>gojobs<span class="dot">.</span>ca</h1>
    <p class="tagline">Go roles in Canada, pulled straight from company ATS boards — no middleman job sites.</p>
    <div class="controls">
      <input id="q" type="search" placeholder="filter by title, company, city…" autofocus>
      <label class="toggle"><input type="checkbox" id="remoteOnly"> remote only</label>
      <label class="field">location
        <select id="region" class="sel"><option value="">all</option></select>
      </label>
      <label class="field">company
        <select id="company" class="sel"><option value="">all</option></select>
      </label>
      <label class="field">source
        <select id="source" class="sel"><option value="">all</option></select>
      </label>
      <span class="field exp-field">exp
        <input id="expMin" type="number" min="0" max="30" placeholder="min" aria-label="minimum years of experience">
        <span class="dash">–</span>
        <input id="expMax" type="number" min="0" max="30" placeholder="max" aria-label="maximum years of experience">
        yrs
      </span>
      <label class="field">sort
        <select id="sort" class="sel">
          <option value="newest">newest</option>
          <option value="oldest">oldest</option>
          <option value="company">company A→Z</option>
          <option value="title">title A→Z</option>
        </select>
      </label>
    </div>
    <div class="meta"><span id="shown">{{.Count}}</span> / {{.Count}} roles · scanned {{.Generated}}</div>
  </header>

  {{if .Jobs}}
  <ul id="list">
    {{range .Jobs}}
    <li class="job" data-remote="{{.Remote}}"
        data-company="{{.Company}}" data-source="{{.Source}}"
        data-region="{{region .Location}}" data-years="{{.MinYears}}"
        data-title="{{.Title}}" data-posted="{{unix .PostedAt}}"
        data-search="{{.Title}} {{.Company}} {{.Location}}">
      <div class="row1">
        <a class="title" href="{{.URL}}" target="_blank" rel="noopener">{{.Title}}</a>
        <span class="tag age">{{since .PostedAt}}</span>
      </div>
      <span class="co">{{.Company}}</span>
      <div class="tags">
        {{if .Location}}<span class="tag loc">{{.Location}}</span>{{end}}
        {{if .Remote}}<span class="tag remote">remote</span>{{end}}
        {{if .MinYears}}<span class="tag exp">{{.MinYears}}+ yrs</span>{{end}}
        <span class="tag">{{.Source}}</span>
      </div>
      {{if .Snippet}}<div class="snip">{{.Snippet}}</div>{{end}}
    </li>
    {{end}}
  </ul>
  <div class="empty" id="noresults" style="display:none">no roles match that filter.</div>
  {{else}}
  <div class="empty">jobs.json is empty. Run <b>gojobs-ca scrape</b> first.</div>
  {{end}}

  <footer>data is only as good as companies.json — edit it to add companies and ATS slugs.</footer>
</div>

<script>
  const q = document.getElementById('q');
  const remoteOnly = document.getElementById('remoteOnly');
  const regionSel = document.getElementById('region');
  const companySel = document.getElementById('company');
  const sourceSel = document.getElementById('source');
  const sortSel = document.getElementById('sort');
  const expMin = document.getElementById('expMin');
  const expMax = document.getElementById('expMax');
  const list = document.getElementById('list');
  const items = Array.from(document.querySelectorAll('li.job'));
  const shown = document.getElementById('shown');
  const noResults = document.getElementById('noresults');

  // Populate the company/source dropdowns from the data that's actually present.
  function fill(sel, values){
    for(const v of [...new Set(values)].sort((a,b)=>a.localeCompare(b))){
      const o = document.createElement('option');
      o.value = v; o.textContent = v;
      sel.appendChild(o);
    }
  }
  fill(regionSel, items.map(li => li.dataset.region).filter(Boolean));
  fill(companySel, items.map(li => li.dataset.company).filter(Boolean));
  fill(sourceSel, items.map(li => li.dataset.source).filter(Boolean));

  const sorters = {
    newest:  (a,b) => (+b.dataset.posted) - (+a.dataset.posted),
    oldest:  (a,b) => (+a.dataset.posted) - (+b.dataset.posted),
    company: (a,b) => a.dataset.company.localeCompare(b.dataset.company)
                      || a.dataset.title.localeCompare(b.dataset.title),
    title:   (a,b) => a.dataset.title.localeCompare(b.dataset.title),
  };

  function apply(){
    const term = q.value.trim().toLowerCase();
    const remote = remoteOnly.checked;
    const region = regionSel.value;
    const company = companySel.value;
    const source = sourceSel.value;
    const lo = expMin.value === '' ? null : parseInt(expMin.value, 10);
    const hi = expMax.value === '' ? null : parseInt(expMax.value, 10);

    // Reorder the DOM to match the chosen sort.
    const ordered = items.slice().sort(sorters[sortSel.value] || sorters.newest);
    for(const li of ordered) list.appendChild(li);

    let n = 0;
    for(const li of ordered){
      const hay = li.dataset.search.toLowerCase();
      const yrs = parseInt(li.dataset.years, 10) || 0;
      const ok = (!term || hay.includes(term))
        && (!remote || li.dataset.remote === 'true')
        && (!region || li.dataset.region === region)
        && (!company || li.dataset.company === company)
        && (!source || li.dataset.source === source)
        && (lo === null || Number.isNaN(lo) || yrs >= lo)
        && (hi === null || Number.isNaN(hi) || yrs <= hi);
      li.style.display = ok ? '' : 'none';
      if(ok) n++;
    }
    if(shown) shown.textContent = n;
    if(noResults) noResults.style.display = (n === 0 && items.length) ? '' : 'none';
  }
  q.addEventListener('input', apply);
  remoteOnly.addEventListener('change', apply);
  regionSel.addEventListener('change', apply);
  companySel.addEventListener('change', apply);
  sourceSel.addEventListener('change', apply);
  sortSel.addEventListener('change', apply);
  expMin.addEventListener('input', apply);
  expMax.addEventListener('input', apply);
  apply();
</script>
</body>
</html>`
