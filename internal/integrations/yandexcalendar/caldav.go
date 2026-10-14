package yandexcalendar

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrAuthorization = errors.New("calendar authorization required")

// Only safe protocol metadata is retained; response bodies may contain private
// calendar data and must not reach logs or public error messages.
type HTTPError struct {
	Operation string
	Status    int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("calendar %s failed (%d)", e.Operation, e.Status)
}

type DAV struct {
	BaseURL string
	Client  *http.Client
}
type davProperty struct {
	Name      string `xml:"displayname"`
	Principal struct {
		Href string `xml:"href"`
	} `xml:"current-user-principal"`
	Home struct {
		Href string `xml:"href"`
	} `xml:"calendar-home-set"`
	Resource struct {
		Calendar *struct{} `xml:"calendar"`
	} `xml:"resourcetype"`
}
type davResponse struct {
	Href  string `xml:"href"`
	Props []struct {
		Status   string      `xml:"status"`
		Property davProperty `xml:"prop"`
	} `xml:"propstat"`
}
type multiStatus struct {
	Responses []davResponse `xml:"response"`
}

func (d *DAV) root() string {
	if d.BaseURL != "" {
		return d.BaseURL
	}
	return "https://caldav.yandex.ru/"
}

// Never follow provider-controlled URLs to another origin or forward a token
// through a redirect. Discovery responses are remote, untrusted input.
func (d *DAV) resolve(href string) (string, error) {
	base, err := url.Parse(d.root())
	if err != nil {
		return "", errors.New("invalid calendar endpoint")
	}
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", errors.New("invalid calendar URL")
	}
	u = base.ResolveReference(u)
	if u.Scheme != base.Scheme || u.Host != base.Host || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("unsafe calendar URL")
	}
	return u.String(), nil
}
func (d *DAV) request(ctx context.Context, method, target, login, token, body, depth string) ([]byte, int, error) {
	raw, status, _, err := d.requestWithLocation(ctx, method, target, login, token, body, depth)
	return raw, status, err
}
func (d *DAV) requestWithLocation(ctx context.Context, method, target, login, token, body, depth string) ([]byte, int, string, error) {
	target, err := d.resolve(target)
	if err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
	if err != nil {
		return nil, 0, "", errors.New("calendar request failed")
	}
	req.SetBasicAuth(login, token)
	if method == "PUT" {
		req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
	} else {
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	}
	if depth != "" {
		req.Header.Set("Depth", depth)
	}
	client := http.Client{Timeout: 12 * time.Second}
	if d.Client != nil {
		client = *d.Client
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return nil, 0, "", errors.New("calendar unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return nil, res.StatusCode, "", ErrAuthorization
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, res.StatusCode, "", errors.New("invalid calendar response")
	}
	return raw, res.StatusCode, res.Header.Get("Location"), nil
}
func (d *DAV) properties(ctx context.Context, target, login, token, props, depth string) ([]davResponse, error) {
	body := `<?xml version="1.0" encoding="UTF-8"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop>` + props + `</d:prop></d:propfind>`
	raw, status, err := d.request(ctx, "PROPFIND", target, login, token, body, depth)
	if err != nil {
		return nil, err
	}
	if status != 207 {
		return nil, &HTTPError{Operation: "PROPFIND", Status: status}
	}
	var result multiStatus
	if xml.Unmarshal(raw, &result) != nil {
		return nil, errors.New("invalid calendar discovery response")
	}
	return result.Responses, nil
}
func successful(p string) bool { return strings.Contains(p, " 200 ") }

func (d *DAV) findCalendar(responses []davResponse, requested string) (string, error) {
	appCalendars := []string{}
	for _, r := range responses {
		href, err := d.resolve(r.Href)
		if err != nil {
			return "", err
		}
		calendar := false
		name := ""
		for _, p := range r.Props {
			if successful(p.Status) {
				calendar = calendar || p.Property.Resource.Calendar != nil
				if p.Property.Name != "" {
					name = p.Property.Name
				}
			}
		}
		if !calendar {
			continue
		}
		if strings.TrimRight(href, "/") == strings.TrimRight(requested, "/") {
			return strings.TrimRight(href, "/") + "/", nil
		}
		if name == "Ритм" {
			appCalendars = append(appCalendars, strings.TrimRight(href, "/")+"/")
		}
	}
	if len(appCalendars) == 1 {
		return appCalendars[0], nil
	}
	if len(appCalendars) > 1 {
		return "", errors.New("multiple Rhythm calendars found")
	}
	return "", nil
}

func (d *DAV) EnsureCalendar(ctx context.Context, login, token, userID string) (string, error) {
	responses, err := d.properties(ctx, d.root(), login, token, `<d:current-user-principal/>`, "0")
	if err != nil {
		return "", err
	}
	principal := ""
	for _, r := range responses {
		for _, p := range r.Props {
			if successful(p.Status) {
				principal = p.Property.Principal.Href
				if principal != "" {
					break
				}
			}
		}
	}
	if principal == "" {
		return "", errors.New("calendar principal missing")
	}
	responses, err = d.properties(ctx, principal, login, token, `<c:calendar-home-set/>`, "0")
	if err != nil {
		return "", err
	}
	home := ""
	for _, r := range responses {
		for _, p := range r.Props {
			if successful(p.Status) && p.Property.Home.Href != "" {
				home = p.Property.Home.Href
			}
		}
	}
	if home == "" {
		return "", errors.New("calendar home missing")
	}
	home, err = d.resolve(home)
	if err != nil {
		return "", err
	}
	target := strings.TrimRight(home, "/") + "/rhythm-" + url.PathEscape(userID) + "/"
	responses, err = d.properties(ctx, home, login, token, `<d:resourcetype/><d:displayname/>`, "1")
	if err != nil {
		return "", err
	}
	if found, err := d.findCalendar(responses, target); err != nil || found != "" {
		return found, err
	}
	body := `<?xml version="1.0" encoding="UTF-8"?><c:mkcalendar xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set><d:prop><d:displayname>Ритм</d:displayname><c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set></d:prop></d:set></c:mkcalendar>`
	_, status, location, err := d.requestWithLocation(ctx, "MKCALENDAR", target, login, token, body, "")
	if err != nil {
		return "", err
	}
	if status != 201 {
		return "", &HTTPError{Operation: "MKCALENDAR", Status: status}
	}
	// Yandex assigns an events-N collection rather than retaining the requested
	// path. Prefer Location, then rediscover the actual calendar after creation.
	if location != "" {
		canonical, err := d.resolve(location)
		if err != nil {
			return "", err
		}
		responses, err = d.properties(ctx, canonical, login, token, `<d:resourcetype/>`, "0")
		if err != nil {
			return "", err
		}
		if found, err := d.findCalendar(responses, canonical); err != nil || found != "" {
			return found, err
		}
	}
	responses, err = d.properties(ctx, home, login, token, `<d:resourcetype/><d:displayname/>`, "1")
	if err != nil {
		return "", err
	}
	found, err := d.findCalendar(responses, target)
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", errors.New("created calendar missing from discovery")
	}
	return found, nil
}
func (d *DAV) Put(ctx context.Context, target, login, token, body string) error {
	_, status, err := d.request(ctx, "PUT", target, login, token, body, "")
	if err != nil {
		return err
	}
	if status != 201 && status != 204 && status != 200 {
		return &HTTPError{Operation: "PUT", Status: status}
	}
	return nil
}
func (d *DAV) Delete(ctx context.Context, target, login, token string) error {
	_, status, err := d.request(ctx, "DELETE", target, login, token, "", "")
	if err != nil {
		return err
	}
	if status != 204 && status != 200 && status != 404 && status != 410 {
		return &HTTPError{Operation: "DELETE", Status: status}
	}
	return nil
}
