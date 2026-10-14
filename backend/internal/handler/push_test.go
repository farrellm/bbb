package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPreferences(t *testing.T) {
	e := setup(t)
	c := e.browser()
	e.signIn(c, "invite="+e.invite(), parent("p"))

	var p preferencesJSON
	e.getJSON(c, "/api/preferences", http.StatusOK, &p)
	if strings.Join(p.Divisions, ",") != "all,lower,middle,upper" || !p.NotifyNew || !p.NotifyEdit ||
		p.NotifyRemoved || p.SignupsOnly || p.SignupsAnyDivision || p.MinEditWords != 1 ||
		p.QuietStart != nil || p.QuietEnd != nil {
		t.Errorf("defaults = %+v", p)
	}

	put := `{"divisions": ["upper", "lower", "upper"], "notifyNew": true, "notifyEdit": false,
		"notifyRemoved": true, "signupsOnly": false, "signupsAnyDivision": true, "minEditWords": 5}`
	decode(t, e.do(c, http.MethodPut, "/api/preferences", put), http.StatusOK, &p)
	if strings.Join(p.Divisions, ",") != "lower,upper" || p.NotifyEdit || !p.SignupsAnyDivision || p.MinEditWords != 5 {
		t.Errorf("saved = %+v", p)
	}
	var again preferencesJSON
	e.getJSON(c, "/api/preferences", http.StatusOK, &again)
	if strings.Join(again.Divisions, ",") != "lower,upper" || again.MinEditWords != 5 || !again.NotifyRemoved {
		t.Errorf("reloaded = %+v", again)
	}

	quiet := `{"divisions": [], "notifyNew": true, "notifyEdit": true, "notifyRemoved": false,
		"signupsOnly": false, "signupsAnyDivision": false, "minEditWords": 1,
		"quietStart": "21:30", "quietEnd": "07:00"}`
	decode(t, e.do(c, http.MethodPut, "/api/preferences", quiet), http.StatusOK, &p)
	e.getJSON(c, "/api/preferences", http.StatusOK, &again)
	if again.QuietStart == nil || *again.QuietStart != "21:30" || again.QuietEnd == nil || *again.QuietEnd != "07:00" {
		t.Errorf("quiet hours = %v, %v", again.QuietStart, again.QuietEnd)
	}

	for _, bad := range []string{
		`{"divisions": ["nope"], "minEditWords": 1}`,
		`{"divisions": [], "minEditWords": -1}`,
		`{"minEditWords": 1}`,
		`{"divisions": [], "extra": 1}`,
		`{"divisions": [], "quietStart": "21:30"}`,
		`{"divisions": [], "quietStart": "21:30", "quietEnd": "21:30"}`,
		`{"divisions": [], "quietStart": "9:30", "quietEnd": "07:00"}`,
		`{"divisions": [], "quietStart": "24:00", "quietEnd": "07:00"}`,
	} {
		decode(t, e.do(c, http.MethodPut, "/api/preferences", bad), http.StatusBadRequest, nil)
	}
}

func TestPushSubscribe(t *testing.T) {
	e := setup(t)
	var key vapidKeyJSON
	e.getJSON(e.browser(), "/api/push/vapid-public-key", http.StatusOK, &key)
	if key.Key == "" {
		t.Fatal("no VAPID key")
	}

	c := e.browser()
	e.signIn(c, "invite="+e.invite(), parent("p"))
	sub := e.push.Subscribe()
	body, _ := json.Marshal(sub)

	var dev deviceJSON
	decode(t, e.do(c, http.MethodPost, "/api/push/subscribe", string(body)), http.StatusCreated, &dev)
	if dev.Endpoint != sub.Endpoint || dev.LastOKAt != nil {
		t.Errorf("device = %+v", dev)
	}
	// Subscribing again (the app does on every launch) keeps one device.
	decode(t, e.do(c, http.MethodPost, "/api/push/subscribe", string(body)), http.StatusCreated, nil)
	var devs []deviceJSON
	e.getJSON(c, "/api/push/devices", http.StatusOK, &devs)
	if len(devs) != 1 {
		t.Fatalf("devices = %+v", devs)
	}

	var res testPushJSON
	decode(t, e.do(c, http.MethodPost, "/api/push/test", ""), http.StatusOK, &res)
	if res.Sent != 1 || res.Failed != 0 {
		t.Errorf("test push = %+v", res)
	}
	if got := e.push.Pushes(); len(got) != 1 || got[0].Payload.Title != "Notifications are on" {
		t.Errorf("pushes = %+v", got)
	}
	e.getJSON(c, "/api/push/devices", http.StatusOK, &devs)
	if devs[0].LastOKAt == nil {
		t.Error("lastOkAt not set after a test push")
	}

	// Another parent signing in on the same device takes the subscription.
	other := e.browser()
	e.signIn(other, "invite="+e.invite(), parent("q"))
	decode(t, e.do(other, http.MethodPost, "/api/push/subscribe", string(body)), http.StatusCreated, nil)
	e.getJSON(c, "/api/push/devices", http.StatusOK, &devs)
	if len(devs) != 0 {
		t.Errorf("first parent still has %d devices", len(devs))
	}
	// Removing someone else's device is a 404.
	decode(t, e.do(c, http.MethodDelete, "/api/push/devices/"+itoa(dev.ID), ""), http.StatusNotFound, nil)

	decode(t, e.do(other, http.MethodDelete, "/api/push/subscribe", `{"endpoint": "`+sub.Endpoint+`"}`),
		http.StatusNoContent, nil)
	e.getJSON(other, "/api/push/devices", http.StatusOK, &devs)
	if len(devs) != 0 {
		t.Errorf("unsubscribed, still %d devices", len(devs))
	}

	second := e.push.Subscribe()
	body, _ = json.Marshal(second)
	decode(t, e.do(c, http.MethodPost, "/api/push/subscribe", string(body)), http.StatusCreated, &dev)
	decode(t, e.do(c, http.MethodDelete, "/api/push/devices/"+itoa(dev.ID), ""), http.StatusNoContent, nil)
}

func TestPushSubscribeRejects(t *testing.T) {
	e := setup(t)
	c := e.browser()
	e.signIn(c, "invite="+e.invite(), parent("p"))
	good := e.push.Subscribe()

	for name, mutate := range map[string]func(*map[string]any){
		"http endpoint": func(m *map[string]any) { (*m)["endpoint"] = "http://web.push.apple.com/x" },
		"unknown host":  func(m *map[string]any) { (*m)["endpoint"] = "https://192.168.1.1/x" },
		"short p256dh": func(m *map[string]any) {
			(*m)["keys"] = map[string]string{"p256dh": "AAAA", "auth": good.Keys.Auth}
		},
		"extra field": func(m *map[string]any) { (*m)["expirationTime"] = nil },
	} {
		b, _ := json.Marshal(good)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mutate(&m)
		b, _ = json.Marshal(m)
		if res := e.do(c, http.MethodPost, "/api/push/subscribe", string(b)); res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, res.StatusCode)
		}
	}
}
