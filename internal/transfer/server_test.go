package transfer

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/thirteenth-order/kh3-save-editor/internal/fixture"
	"github.com/thirteenth-order/kh3-save-editor/internal/kh3"
)

const testSteamID = "76561190000000001"

func writeSyntheticSet(t *testing.T, dir, account string, slots map[int][]byte) map[string][]byte {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	key, err := kh3.DeriveKey(account)
	if err != nil {
		t.Fatal(err)
	}
	written := map[string][]byte{}
	for slot, plain := range slots {
		name := fmtSlot(slot)
		blob, err := kh3.Wrap(kh3.PadToBlock(plain), key)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), blob, 0o600); err != nil {
			t.Fatal(err)
		}
		written[name] = blob
	}
	return written
}
func fmtSlot(n int) string { return "KHIII_slot" + string(rune('0'+n)) + ".bin" }

func writeSyntheticSystem(t *testing.T, dir string) ([]byte, []byte) {
	t.Helper()
	plain := make([]byte, 0x7B20)
	copy(plain, kh3.Magic)
	binary.LittleEndian.PutUint32(plain[4:], 0x7B08)
	binary.LittleEndian.PutUint16(plain[8:], 5)
	binary.LittleEndian.PutUint16(plain[10:], 2)
	sealed, err := kh3.Seal(plain, kh3.FormatPlain, nil)
	if err != nil {
		t.Fatal(err)
	}
	key, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := kh3.Wrap(sealed, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "KHIII_system.bin"), blob, 0o600); err != nil {
		t.Fatal(err)
	}
	return sealed, blob
}

func callAPI(t *testing.T, s *Server, method, route string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&b).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://"+s.addr+route+"?t="+s.token, &b)
	req.Host = s.addr
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

func TestEpicToSteamFlowUsesVerifiedUpstreamContainerAndVisibleBackups(t *testing.T) {
	root := t.TempDir()
	gameRoot := filepath.Join(root, "KINGDOM HEARTS III")
	epic := filepath.Join(gameRoot, "Epic Games Store", kh3.EpicAccount, "SaveGames", "kh3sv2", "data")
	steam := filepath.Join(root, "KINGDOM HEARTS III", "Steam", testSteamID, "SaveGames", "kh3sv2", "data")
	epicOriginals := writeSyntheticSet(t, epic, kh3.EpicAccount, fixture.Slots())
	systemPlain, systemBlob := writeSyntheticSystem(t, epic)
	epicOriginals["KHIII_system.bin"] = systemBlob
	steamOriginals := writeSyntheticSet(t, steam, testSteamID, map[int][]byte{1: fixture.Build(fixture.Options{Difficulty: 2, Level: 20, HP: 99, MP: 80})})

	if !ParseSteamID(testSteamID) || ParseSteamID("1") || ParseSteamID("7656119000000000x") {
		t.Fatal("SteamID64 validation accepted an invalid ID")
	}
	files, err := scanEpic(gameRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("scan found %d files, want three synthetic slots and the system file", len(files))
	}
	dest, gotID, err := resolveSteamDestination(steam, "")
	if err != nil {
		t.Fatal(err)
	}
	if dest != steam || gotID != testSteamID {
		t.Fatalf("destination = %q / %q", dest, gotID)
	}
	steamRoot := filepath.Join(root, "KINGDOM HEARTS III", "Steam")
	if err := os.MkdirAll(filepath.Join(steamRoot, testSteamID), 0o755); err != nil {
		t.Fatal(err)
	}
	rootDest, rootID, err := resolveSteamDestination(steamRoot, "")
	if err != nil || rootID != testSteamID || rootDest != filepath.Join(steamRoot, testSteamID, "SaveGames", "kh3sv2", "data") {
		t.Fatalf("Steam root auto-detection = %q / %q / %v", rootDest, rootID, err)
	}

	s := &Server{token: "test-token", addr: "127.0.0.1:12345", mux: http.NewServeMux(), pending: map[string]*prepared{}}
	s.mux.HandleFunc("/api/review", s.guard(s.review))
	s.mux.HandleFunc("/api/prepare", s.guard(s.prepare))
	s.mux.HandleFunc("/api/install", s.guard(s.install))
	base := reviewRequest{Source: gameRoot, Destination: steam}
	if res := callAPI(t, s, http.MethodPost, "/api/review", base); res.Code != 200 {
		t.Fatalf("review: %d %s", res.Code, res.Body)
	}
	withoutConsent := prepareRequest{reviewRequest: base, ExpectedReplacements: []string{"KHIII_slot1.bin"}}
	if res := callAPI(t, s, http.MethodPost, "/api/prepare", withoutConsent); res.Code != 409 {
		t.Fatalf("prepare without overwrite confirmation: %d %s", res.Code, res.Body)
	}
	withConsent := prepareRequest{reviewRequest: base, Overwrite: true, ExpectedReplacements: []string{"KHIII_slot1.bin"}}
	res := callAPI(t, s, http.MethodPost, "/api/prepare", withConsent)
	if res.Code != 200 {
		t.Fatalf("prepare: %d %s", res.Code, res.Body)
	}
	var ready struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &ready); err != nil || ready.ID == "" {
		t.Fatalf("prepared response: %v %s", err, res.Body)
	}
	installReq := struct {
		ID      string `json:"id"`
		Confirm bool   `json:"confirm"`
	}{ID: ready.ID, Confirm: true}
	res = callAPI(t, s, http.MethodPost, "/api/install", installReq)
	if res.Code != 200 {
		t.Fatalf("install: %d %s", res.Code, res.Body)
	}
	var installed struct {
		Backup string   `json:"backup"`
		Files  []string `json:"files"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	if len(installed.Files) != 4 {
		t.Fatalf("installed %d files", len(installed.Files))
	}
	stages, err := filepath.Glob(filepath.Join(steam, "Preparacion temporal KH3-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("temporary stage should be cleaned after install: %v", stages)
	}
	for name, original := range epicOriginals {
		got, err := os.ReadFile(filepath.Join(epic, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, original) {
			t.Fatalf("Epic original %s was modified", name)
		}
		converted, err := os.ReadFile(filepath.Join(steam, name))
		if err != nil {
			t.Fatal(err)
		}
		opened, err := kh3.OpenFile(filepath.Join(steam, name), testSteamID)
		if err != nil {
			t.Fatalf("Steam output %s failed verification: %v", name, err)
		}
		if opened.Format != kh3.FormatSteam || opened.Account != testSteamID {
			t.Fatalf("%s did not reread as Steam", name)
		}
		var plain []byte
		if name == "KHIII_system.bin" {
			plain = systemPlain
		} else {
			plain = fixture.Slots()[int(name[len("KHIII_slot")]-'0')]
		}
		if !bytes.Equal(opened.Plain[:len(plain)], plain) {
			t.Fatalf("converted plaintext differs for %s", name)
		}
		if bytes.Equal(converted, original) {
			t.Fatalf("%s was not rekeyed (Steam %x, Epic %x)", name, converted[:8], original[:8])
		}
	}
	backupEpic := filepath.Join(installed.Backup, "Epic")
	backupSteam := filepath.Join(installed.Backup, "Steam reemplazado")
	for name, original := range epicOriginals {
		got, err := os.ReadFile(filepath.Join(backupEpic, name))
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("Epic backup %s missing or changed: %v", name, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(backupSteam, "KHIII_slot1.bin")); err != nil || !bytes.Equal(got, steamOriginals["KHIII_slot1.bin"]) {
		t.Fatalf("Steam backup missing or changed: %v", err)
	}
}

func TestScanEpicRejectsPlainConsoleAndForeignFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "KHIII_slot0.bin"), fixture.Build(fixture.Default()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scanEpic(root); err == nil {
		t.Fatal("plain/console KH3 file was accepted as an Epic save")
	}
}

func TestLocalUIAndRequestGuards(t *testing.T) {
	s := &Server{token: "local-test-token", addr: "127.0.0.1:23456", mux: http.NewServeMux(), pending: map[string]*prepared{}}
	s.mux.HandleFunc("/", s.guard(s.index))
	s.mux.HandleFunc("/style.css", s.guard(s.index))
	s.mux.HandleFunc("/app.js", s.guard(s.index))
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:23456/?t=local-test-token", nil)
	req.Host = s.addr
	res := httptest.NewRecorder()
	s.mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !bytes.Contains(res.Body.Bytes(), []byte(`<html lang="es">`)) || !bytes.Contains(res.Body.Bytes(), []byte("Revisa antes de convertir")) {
		t.Fatalf("Spanish local UI did not load: %d", res.Code)
	}
	if !bytes.Contains(res.Body.Bytes(), []byte("style.css?t=local-test-token")) || !bytes.Contains(res.Body.Bytes(), []byte("app.js?t=local-test-token")) {
		t.Fatal("local static files did not receive the per-run token")
	}
	for _, asset := range []string{"style.css", "app.js"} {
		assetReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:23456/"+asset+"?t=local-test-token", nil)
		assetReq.Host = s.addr
		assetRes := httptest.NewRecorder()
		s.mux.ServeHTTP(assetRes, assetReq)
		if assetRes.Code != http.StatusOK {
			t.Fatalf("local asset %s did not load: %d", asset, assetRes.Code)
		}
	}
	for _, tc := range []struct{ host, token, fetchSite string }{
		{"127.0.0.1:23456", "", ""},
		{"attacker.example:23456", "local-test-token", ""},
		{"127.0.0.1:23456", "local-test-token", "cross-site"},
	} {
		bad := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:23456/", nil)
		bad.Host = tc.host
		bad.URL.RawQuery = "t=" + tc.token
		if tc.fetchSite != "" {
			bad.Header.Set("Sec-Fetch-Site", tc.fetchSite)
		}
		out := httptest.NewRecorder()
		s.mux.ServeHTTP(out, bad)
		if out.Code != http.StatusForbidden {
			t.Fatalf("guard accepted request host=%q fetch=%q: %d", tc.host, tc.fetchSite, out.Code)
		}
	}
}

func TestBrowserFilePickerUploadKeepsOnlyVerifiedEpicBytesInMemory(t *testing.T) {
	epicKey, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		t.Fatal(err)
	}
	plain := fixture.Build(fixture.Default())
	blob, err := kh3.Wrap(kh3.PadToBlock(plain), epicKey)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "KHIII_slot0.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(blob); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	s := &Server{token: "browser-file-test", addr: "127.0.0.1:23457", mux: http.NewServeMux(), uploads: map[string][]saveFile{}}
	s.mux.HandleFunc("/api/upload", s.guard(s.upload))
	s.mux.HandleFunc("/api/review", s.guard(s.review))
	s.mux.HandleFunc("/api/prepare", s.guard(s.prepare))
	s.mux.HandleFunc("/api/install", s.guard(s.install))
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:23457/api/upload?t=browser-file-test", &body)
	req.Host = s.addr
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-KH3-Token", s.token)
	res := httptest.NewRecorder()
	s.mux.ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("browser file upload: %d %s", res.Code, res.Body)
	}
	var response struct {
		UploadID string `json:"uploadId"`
		Files    []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.UploadID == "" || len(response.Files) != 1 || response.Files[0].Name != "KHIII_slot0.bin" {
		t.Fatalf("unexpected file selection: %+v", response)
	}
	selected, err := s.sourceFiles(reviewRequest{UploadID: response.UploadID})
	if err != nil || len(selected) != 1 || !bytes.Equal(selected[0].Data, blob) {
		t.Fatalf("local selection not retained correctly: %v", err)
	}
	if selected[0].Path != "" {
		t.Fatal("browser selection should be held in memory, not copied to a persistent source path")
	}
	steam := filepath.Join(t.TempDir(), "Steam", testSteamID, "SaveGames", "kh3sv2", "data")
	reqData := reviewRequest{UploadID: response.UploadID, Destination: steam}
	res = callAPI(t, s, http.MethodPost, "/api/review", reqData)
	if res.Code != 200 {
		t.Fatalf("review uploaded file: %d %s", res.Code, res.Body)
	}
	prepareData := prepareRequest{reviewRequest: reqData, ExpectedReplacements: []string{}}
	res = callAPI(t, s, http.MethodPost, "/api/prepare", prepareData)
	if res.Code != 200 {
		t.Fatalf("prepare uploaded file: %d %s", res.Code, res.Body)
	}
	var ready struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &ready); err != nil || ready.ID == "" {
		t.Fatalf("prepared upload response: %v", err)
	}
	res = callAPI(t, s, http.MethodPost, "/api/install", struct {
		ID      string `json:"id"`
		Confirm bool   `json:"confirm"`
	}{ID: ready.ID, Confirm: true})
	if res.Code != 200 {
		t.Fatalf("install uploaded file: %d %s", res.Code, res.Body)
	}
	var installed struct {
		Backup string `json:"backup"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &installed); err != nil {
		t.Fatal(err)
	}
	converted, err := kh3.OpenFile(filepath.Join(steam, "KHIII_slot0.bin"), testSteamID)
	if err != nil || converted.Account != testSteamID || !bytes.Equal(converted.Plain[:len(plain)], plain) {
		t.Fatalf("uploaded output did not validate as the Steam account: %v", err)
	}
	backup, err := os.ReadFile(filepath.Join(installed.Backup, "Epic", "KHIII_slot0.bin"))
	if err != nil || !bytes.Equal(backup, blob) {
		t.Fatalf("uploaded original was not backed up exactly: %v", err)
	}
	s.mu.Lock()
	_, retained := s.uploads[response.UploadID]
	s.mu.Unlock()
	if retained {
		t.Fatal("uploaded source bytes should be released after installation")
	}
}
