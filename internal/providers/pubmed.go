package providers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// pubmedProvider talks to NCBI E-utilities. It is the only two(/three)-step
// driver: esearch (JSON → PMIDs) → esummary (JSON → title/source/pubdate) and,
// when fetch_abstracts is on, efetch (XML → abstracts). Every upstream call is
// reported through the ctx StepReporter so the history shows one row per stage
// while the node stays one logical unit.
//
// NCBI rate limits: 3 rps without an API key, 10 rps with one — enforced by a
// per-driver token bucket shared across all nodes of this driver.
type pubmedProvider struct {
	http *HTTPClient
}

// Code returns the provider identifier.
func (p pubmedProvider) Code() string { return "pubmed" }

// Schema describes the provider configuration UI.
func (p pubmedProvider) Schema() ProviderSchema {
	return ProviderSchema{
		Code:        "pubmed",
		Name:        "PubMed (NCBI)",
		Credentials: credentialFields("api_key"),
		Params: []ParamField{
			numberParam("retmax", "Max results", "10", "esearch retmax"),
			selectParam("sort", "Sort", "relevance", ParamOption{"Relevance", "relevance"}, ParamOption{"Pub date", "pub date"}),
			boolParam("fetch_abstracts", "Fetch abstracts", false, "Adds a third efetch call per request"),
			numberParam("snippet_max_chars", "Snippet max chars", "600", "Abstracts are truncated to this length"),
			textParam("tool", "NCBI tool name", "serpentseek", "Polite identification, recommended by NCBI"),
			textParam("email", "NCBI contact e-mail", "", "Recommended by NCBI"),
			numberParam("max_rps", "Max requests/sec", "", "Default 3 without api_key, 10 with one"),
			textParam("fatal_http", "Fatal HTTP codes", "400", ""),
			textParam("retry_http_codes", "Retry HTTP codes", "429,500,502,503,504", ""),
		},
		Hints: []string{
			"Two-step NCBI E-utilities flow (esearch → esummary); each upstream call is a separate history step.",
			"An empty PMID list short-circuits the flow to `empty` — the second call is skipped.",
		},
	}
}

// pubmedEsearch mirrors the esearch JSON envelope.
type pubmedEsearch struct {
	Result struct {
		IDList []string `json:"idlist"`
		Error  string   `json:"error"`
	} `json:"esearchresult"`
}

// pubmedAbstractText captures both the Label attribute and the inner text of
// an <AbstractText> element (which may contain nested formatting tags).
type pubmedAbstractText struct {
	Label string
	Text  string
}

// UnmarshalXML concatenates character data and remembers the Label attribute.
func (a *pubmedAbstractText) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, attr := range start.Attr {
		if attr.Name.Local == "Label" {
			a.Label = attr.Value
		}
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.CharData:
			a.Text += string(tt)
		case xml.EndElement:
			if tt.Name == start.Name {
				return nil
			}
		}
	}
}

// pubmedFetchSet mirrors the efetch XML envelope.
type pubmedFetchSet struct {
	XMLName  xml.Name `xml:"PubmedArticleSet"`
	XMLError string   `xml:"ERROR"`
	Articles []struct {
		PMID      string               `xml:"MedlineCitation>PMID"`
		Abstracts []pubmedAbstractText `xml:"MedlineCitation>Article>Abstract>AbstractText"`
	} `xml:"PubmedArticle"`
}

func (p pubmedProvider) Search(ctx context.Context, q Query, c Credentials, params Params) Result {
	base := strings.TrimRight(defaultStr(params["base_url"], "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"), "/")
	key := strings.TrimSpace(c["api_key"])
	if key == "" {
		key = strings.TrimSpace(params["api_key"])
	}
	fatalHTTP := parseIntCSV(params["fatal_http"], []int{400})
	retryHTTP := parseIntCSV(params["retry_http_codes"], commonRetry())

	// NCBI: 3 rps anonymous, 10 rps with an API key; max_rps param overrides.
	defRPS := 3.0
	if key != "" {
		defRPS = 10
	}
	rps := maxRPSParam(params, defRPS)

	// get performs one throttled E-utilities call and reports the stage.
	get := func(stage, path string, values url.Values) (int, []byte, error) {
		if key != "" {
			values.Set("api_key", key)
		}
		if v := strings.TrimSpace(params["tool"]); v != "" {
			values.Set("tool", v)
		}
		if v := strings.TrimSpace(params["email"]); v != "" {
			values.Set("email", v)
		}
		endpoint := base + path + "?" + values.Encode()
		t0 := time.Now()
		status := 0
		var out []byte
		err := func() error {
			if err := WaitLimit(ctx, p.Code(), rps); err != nil {
				return err
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
			if err != nil {
				return err
			}
			resp, err := p.http.Do(req)
			if err != nil {
				return err
			}
			status = resp.StatusCode
			body, _, readErr := p.http.ReadBody(resp)
			out = body
			return readErr
		}()
		ReportStage(ctx, stage, status, int(time.Since(t0)/time.Millisecond), err)
		return status, out, err
	}

	retmax := defaultStr(params["retmax"], "10")
	if q.Count > 0 {
		retmax = strconv.Itoa(q.Count)
	}

	// --- Stage 1: esearch → PMIDs ---
	searchValues := url.Values{
		"db": {"pubmed"}, "term": {q.Text}, "retmode": {"json"},
		"retmax": {retmax}, "sort": {defaultStr(params["sort"], "relevance")},
	}
	status, body, err := get("esearch", "/esearch.fcgi", searchValues)
	if res, done := p.classifyStage(err, status, body, fatalHTTP, retryHTTP, "esearch"); done {
		return res
	}
	var found pubmedEsearch
	if err := jsonUnmarshal(body, &found); err != nil {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: status,
			Error: "pubmed: esearch JSON parse failed: " + err.Error()}
	}
	if msg := strings.TrimSpace(found.Result.Error); msg != "" {
		return Result{Kind: KindAPI, Permanent: false, Provider: p.Code(), HTTPStatus: status,
			Error: "pubmed: esearch error: " + p.http.Redact(msg)}
	}
	idlist := found.Result.IDList
	if len(idlist) == 0 {
		// Nothing found: the second call is skipped on purpose (quota economy).
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}

	// --- Stage 2: esummary → per-UID summaries ---
	summaryValues := url.Values{
		"db": {"pubmed"}, "id": {strings.Join(idlist, ",")}, "retmode": {"json"},
	}
	status, body, err = get("esummary", "/esummary.fcgi", summaryValues)
	if res, done := p.classifyStage(err, status, body, fatalHTTP, retryHTTP, "esummary"); done {
		return res
	}
	var env map[string]any
	if err := jsonUnmarshal(body, &env); err != nil {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: status,
			Error: "pubmed: esummary JSON parse failed: " + err.Error()}
	}
	resultBlock, _ := env["result"].(map[string]any)
	if resultBlock == nil {
		return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: status,
			Error: "pubmed: esummary response has no result block"}
	}
	var uids []string
	if arr, ok := resultBlock["uids"].([]any); ok {
		for _, u := range arr {
			if s := asString(u); s != "" {
				uids = append(uids, s)
			}
		}
	}
	if len(uids) == 0 {
		return Result{OK: true, Kind: KindEmpty, Rows: nil, Provider: p.Code(), HTTPStatus: status}
	}

	rows := make(Rows, 0, len(uids))
	for _, uid := range uids {
		doc, _ := resultBlock[uid].(map[string]any)
		if doc == nil {
			continue
		}
		parts := []string{}
		if s := asString(doc["source"]); s != "" {
			parts = append(parts, s)
		}
		if s := asString(doc["pubdate"]); s != "" {
			parts = append(parts, s)
		}
		rows = append(rows, Row{
			Link:    "https://pubmed.ncbi.nlm.nih.gov/" + uid + "/",
			Title:   asString(doc["title"]),
			Snippet: strings.Join(parts, " · "),
		})
	}

	// --- Stage 3 (optional): efetch → abstracts as snippets ---
	if params["fetch_abstracts"] == "true" && len(rows) > 0 {
		maxChars := 600
		if v, err := strconv.Atoi(strings.TrimSpace(params["snippet_max_chars"])); err == nil && v > 0 {
			maxChars = v
		}
		fetchValues := url.Values{
			"db": {"pubmed"}, "id": {strings.Join(idlist, ",")}, "retmode": {"xml"},
		}
		status, body, err = get("efetch", "/efetch.fcgi", fetchValues)
		if res, done := p.classifyStage(err, status, body, fatalHTTP, retryHTTP, "efetch"); done {
			return res
		}
		var fetched pubmedFetchSet
		if err := xml.Unmarshal(body, &fetched); err != nil {
			return Result{Kind: KindJSON, Provider: p.Code(), HTTPStatus: status,
				Error: "pubmed: efetch XML parse failed: " + err.Error()}
		}
		if msg := strings.TrimSpace(fetched.XMLError); msg != "" {
			return Result{Kind: KindAPI, Permanent: false, Provider: p.Code(), HTTPStatus: status,
				Error: "pubmed: efetch error: " + p.http.Redact(msg)}
		}
		abstracts := map[string]string{}
		for _, art := range fetched.Articles {
			parts := []string{}
			for _, ab := range art.Abstracts {
				text := strings.TrimSpace(ab.Text)
				if text == "" {
					continue
				}
				if ab.Label != "" {
					text = ab.Label + ": " + text
				}
				parts = append(parts, text)
			}
			abstracts[art.PMID] = strings.Join(parts, " ")
		}
		for i := range rows {
			pmid := strings.TrimSuffix(strings.TrimPrefix(rows[i].Link, "https://pubmed.ncbi.nlm.nih.gov/"), "/")
			if abs := abstracts[pmid]; abs != "" {
				rows[i].Snippet = truncateString(abs, maxChars)
			}
		}
	}

	return Result{OK: true, Kind: KindOK, Rows: rows, Provider: p.Code(), HTTPStatus: status}
}

// classifyStage maps a failed stage to a Result; done=false means “proceed”.
func (p pubmedProvider) classifyStage(err error, status int, body []byte, fatalHTTP, retryHTTP []int, stage string) (Result, bool) {
	if err != nil {
		return Result{Kind: ClassifyTransport(err), Provider: p.Code(),
			Error: fmt.Sprintf("pubmed: %s: %s", stage, p.http.Redact(err.Error()))}, true
	}
	if !IsJSONStatus(status) {
		kind, permanent := HTTPClassify(status, retryHTTP)
		if containsInt(fatalHTTP, status) {
			permanent = true
		}
		return Result{Kind: kind, Permanent: permanent, Provider: p.Code(), HTTPStatus: status,
			Error: fmt.Sprintf("pubmed: %s: %s", stage, p.http.HTTPError(status, "", body))}, true
	}
	return Result{}, false
}

// jsonUnmarshal is a thin wrapper keeping the call sites tidy.
func jsonUnmarshal(body []byte, out any) error {
	return json.Unmarshal(body, out)
}
