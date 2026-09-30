// Package transfer provides a local-only, guided Epic-to-Steam migration UI.
// Save parsing, key derivation, conversion, and integrity checks all come from
// the GPL-3.0 kh3 package in this repository.
// Added 2026-09-29 to upstream v0.2.1; see the repository LICENSE.
package transfer

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/thirteenth-order/kh3-save-editor/internal/kh3"
)

//go:embed assets
var assets embed.FS

var slotName = regexp.MustCompile(`^KHIII_(?:slot[0-9]+|system)\.bin$`)
var steamID = regexp.MustCompile(`^7656119[0-9]{10}$`)

type Server struct {
	token   string
	addr    string
	mux     *http.ServeMux
	mu      sync.Mutex
	pending map[string]*prepared
	uploads map[string][]saveFile
}

type saveFile struct {
	Path string `json:"-"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Hash string `json:"-"`
	Data []byte `json:"-"`
}
type reviewRequest struct {
	Source      string `json:"source"`
	UploadID    string `json:"uploadId"`
	Destination string `json:"destination"`
	Account     string `json:"account"`
}
type reviewResponse struct {
	Source       string     `json:"source"`
	Destination  string     `json:"destination"`
	Account      string     `json:"account"`
	Files        []saveFile `json:"files"`
	Replacements []string   `json:"replacements"`
}
type prepareRequest struct {
	reviewRequest
	Overwrite            bool     `json:"overwrite"`
	ExpectedReplacements []string `json:"expectedReplacements"`
}
type prepared struct {
	Destination string
	Source      string
	UploadID    string
	Account     string
	Files       []saveFile
	Stage       string
	Replace     []bool
	SteamHashes []string
	CreatedAt   time.Time
}

func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, s.addr) {
			http.Error(w, "host local no permitido", http.StatusForbidden)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.Header.Get("Sec-Fetch-Site") == "same-site" {
			http.Error(w, "solicitud entre sitios rechazada", http.StatusForbidden)
			return
		}
		got := r.Header.Get("X-KH3-Token")
		if got == "" {
			got = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			http.Error(w, "token local incorrecto", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self'; form-action 'none'; frame-ancestors 'none'; base-uri 'none'")
		next(w, r)
	}
}

func hostAllowed(host, addr string) bool {
	if host == addr {
		return true
	}
	h, p, err := net.SplitHostPort(host)
	if err != nil {
		return false
	}
	_, want, err := net.SplitHostPort(addr)
	return err == nil && p == want && (h == "127.0.0.1" || h == "localhost" || h == "::1")
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	jsonOut(w, status, map[string]string{"error": spanishError(err)})
}

func spanishError(err error) string {
	msg := err.Error()
	for _, pair := range [][2]string{
		{"trailing MD5 mismatch; file is corrupt or truncated", "el MD5 final no coincide; el archivo está dañado o incompleto"},
		{"decrypted data does not start with 'S@vE'; wrong account id or key", "el archivo no se reconoce con la clave de Epic; revisa que sea un guardado de KH III"},
		{"does not decrypt", "no se puede descifrar con el identificador de Epic"},
		{"CRC32 mismatch", "el CRC32 no coincide; el archivo está dañado"},
		{"unexpected trailer separator", "el marcador final del archivo no es válido"},
		{"ciphertext length", "la longitud cifrada del archivo es inválida"},
		{"implausible filesize field", "el tamaño interno del archivo no es válido"},
		{"file too short", "el archivo está incompleto"},
		{"The system cannot find the path specified.", "No se encontró la carpeta o el archivo."},
		{"Access is denied.", "Windows denegó el acceso; comprueba los permisos de la carpeta."},
		{"permission denied", "acceso denegado; comprueba los permisos de la carpeta"},
		{"no such file or directory", "no se encontró la carpeta o el archivo"},
	} {
		msg = strings.ReplaceAll(msg, pair[0], pair[1])
	}
	return msg
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Method != http.MethodPost {
		return errors.New("se esperaba POST")
	}
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(v)
}
func randomHex(n int) string {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if r.URL.Path == "/" || name == "." || name == "" {
		name = "index.html"
	}
	if name != "index.html" && name != "app.js" && name != "style.css" {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(assets, "assets/"+name)
	if err != nil {
		http.Error(w, "interfaz no disponible", 500)
		return
	}
	if name == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		b = []byte(strings.ReplaceAll(string(b), "{{TOKEN}}", s.token))
	}
	if name == "app.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	if name == "style.css" {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	_, _ = w.Write(b)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	var out []map[string]string
	for _, d := range kh3.FindSaveDirs() {
		if strings.EqualFold(d.Platform, "Epic Games Store") {
			out = append(out, map[string]string{"path": d.Path, "platform": d.Platform})
		}
	}
	jsonOut(w, 200, map[string]any{"folders": out})
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	if req.Kind != "epic" && req.Kind != "steam" {
		fail(w, 400, errors.New("tipo de carpeta desconocido"))
		return
	}
	prompt := "Elige la carpeta de guardados de Epic Games Store de KINGDOM HEARTS III"
	if req.Kind == "steam" {
		prompt = "Elige la carpeta de destino de Steam para KINGDOM HEARTS III"
	}
	p, err := pickFolder(prompt)
	if err != nil {
		fail(w, 400, err)
		return
	}
	jsonOut(w, 200, map[string]string{"path": p})
}

// upload receives the user's FileList over the loopback-only server and keeps
// the bytes in memory. It creates no persistent source copy on disk.
func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, errors.New("se esperaba POST"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, errors.New("no pude leer la selección de archivos del navegador"))
		return
	}
	var files []saveFile
	seen := map[string]bool{}
	total := int64(0)
	key, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		fail(w, 500, err)
		return
	}
	for {
		part, e := mr.NextPart()
		if e == io.EOF {
			break
		}
		if e != nil {
			fail(w, 400, fmt.Errorf("no pude leer los archivos seleccionados: %w", e))
			return
		}
		name := filepath.Base(part.FileName())
		if name == "." || !slotName.MatchString(name) {
			_ = part.Close()
			fail(w, 400, fmt.Errorf("%q no es un archivo compatible. Selecciona KHIII_slot*.bin o KHIII_system.bin", name))
			return
		}
		if seen[name] {
			_ = part.Close()
			fail(w, 400, fmt.Errorf("seleccionaste %s más de una vez", name))
			return
		}
		if len(files) >= 16 {
			_ = part.Close()
			fail(w, 413, errors.New("seleccionaste más de 16 archivos; elige solo los guardados de KH III"))
			return
		}
		b, e := io.ReadAll(io.LimitReader(part, 20<<20+1))
		_ = part.Close()
		if e != nil {
			fail(w, 400, fmt.Errorf("no pude leer %s: %w", name, e))
			return
		}
		if len(b) > 20<<20 {
			fail(w, 413, fmt.Errorf("%s supera el tamaño esperado para un guardado de KH III", name))
			return
		}
		total += int64(len(b))
		if total > 200<<20 {
			fail(w, 413, errors.New("la selección supera 200 MB; selecciona solo los archivos de guardado de KH III"))
			return
		}
		_, format, e := kh3.Open(b, key)
		if e != nil {
			fail(w, 400, fmt.Errorf("%s no pasó las comprobaciones de Epic: %w", name, e))
			return
		}
		if format != kh3.FormatSteam {
			fail(w, 400, fmt.Errorf("%s no es un guardado de PC de Epic; no se aceptan archivos de consola", name))
			return
		}
		seen[name] = true
		files = append(files, saveFile{Name: name, Size: int64(len(b)), Hash: hashBytes(b), Data: b})
	}
	if len(files) == 0 {
		fail(w, 400, errors.New("selecciona uno o varios archivos KHIII_slot*.bin o KHIII_system.bin"))
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	id := randomHex(16)
	s.mu.Lock()
	if s.uploads == nil {
		s.uploads = map[string][]saveFile{}
	}
	// This UI has one active browser page; release an earlier file selection
	// when the user makes a new one. Any staged conversion already in progress
	// owns its own immutable slices until it is installed or discarded.
	s.uploads = map[string][]saveFile{id: files}
	s.mu.Unlock()
	jsonOut(w, 200, map[string]any{"uploadId": id, "files": publicFiles(files)})
}

func publicFiles(files []saveFile) []map[string]any {
	out := make([]map[string]any, len(files))
	for i, f := range files {
		out[i] = map[string]any{"name": f.Name, "size": f.Size}
	}
	return out
}

func (s *Server) sourceFiles(req reviewRequest) ([]saveFile, error) {
	if req.UploadID == "" {
		return scanEpic(req.Source)
	}
	s.mu.Lock()
	files, ok := s.uploads[req.UploadID]
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("la selección de archivos expiró; vuelve a elegirlos")
	}
	key, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if !slotName.MatchString(f.Name) || hashBytes(f.Data) != f.Hash {
			return nil, fmt.Errorf("la selección de %s ya no es válida", f.Name)
		}
		_, format, e := kh3.Open(f.Data, key)
		if e != nil {
			return nil, fmt.Errorf("%s no pasó las comprobaciones de Epic: %w", f.Name, e)
		}
		if format != kh3.FormatSteam {
			return nil, fmt.Errorf("%s no es un guardado cifrado de PC de Epic", f.Name)
		}
	}
	return files, nil
}

func (s *Server) releaseUpload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UploadID string `json:"uploadId"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	s.mu.Lock()
	delete(s.uploads, req.UploadID)
	s.mu.Unlock()
	jsonOut(w, 200, map[string]bool{"released": true})
}

func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	var req reviewRequest
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	files, err := s.sourceFiles(req)
	if err != nil {
		fail(w, 400, err)
		return
	}
	dest, account, err := resolveSteamDestination(req.Destination, req.Account)
	if err != nil {
		fail(w, 400, err)
		return
	}
	replacements := make([]string, 0)
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(dest, f.Name)); err == nil {
			replacements = append(replacements, f.Name)
		} else if !errors.Is(err, os.ErrNotExist) {
			fail(w, 400, err)
			return
		}
	}
	jsonOut(w, 200, reviewResponse{Source: req.Source, Destination: dest, Account: account, Files: files, Replacements: replacements})
}

func (s *Server) prepare(w http.ResponseWriter, r *http.Request) {
	var req prepareRequest
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	files, err := s.sourceFiles(req.reviewRequest)
	if err != nil {
		fail(w, 400, err)
		return
	}
	dest, account, err := resolveSteamDestination(req.Destination, req.Account)
	if err != nil {
		fail(w, 400, err)
		return
	}
	key, err := kh3.DeriveKey(account)
	if err != nil {
		fail(w, 400, err)
		return
	}
	sourceKey, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		fail(w, 500, err)
		return
	}
	var replace []bool
	var steamHashes []string
	var replacements []string
	for _, f := range files {
		target := filepath.Join(dest, f.Name)
		_, e := os.Stat(target)
		exists := e == nil
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			fail(w, 400, e)
			return
		}
		if exists && !req.Overwrite {
			fail(w, 409, fmt.Errorf("%s ya existe en Steam; confirma el reemplazo después de revisar", f.Name))
			return
		}
		replace = append(replace, exists)
		hash := ""
		if exists {
			hash, e = fileHash(target)
			if e != nil {
				fail(w, 400, fmt.Errorf("no se pudo leer el destino %s: %w", f.Name, e))
				return
			}
			replacements = append(replacements, f.Name)
		}
		steamHashes = append(steamHashes, hash)
	}
	if !sameStrings(replacements, req.ExpectedReplacements) {
		fail(w, http.StatusConflict, errors.New("el destino cambió desde la revisión; vuelve a revisar los archivos antes de convertir"))
		return
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		fail(w, 500, err)
		return
	}
	stage, err := os.MkdirTemp(dest, "Preparacion temporal KH3-")
	if err != nil {
		fail(w, 500, err)
		return
	}
	newDir := filepath.Join(stage, "Archivos convertidos")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		_ = os.RemoveAll(stage)
		fail(w, 500, err)
		return
	}
	for _, f := range files {
		blob, e := sourceBytes(f)
		if e != nil {
			_ = os.RemoveAll(stage)
			fail(w, 400, fmt.Errorf("no pude leer %s: %w", f.Name, e))
			return
		}
		if hashBytes(blob) != f.Hash {
			_ = os.RemoveAll(stage)
			fail(w, 409, fmt.Errorf("%s cambió después de la revisión; vuelve a revisar", f.Name))
			return
		}
		plain, sourceFormat, e := kh3.Open(blob, sourceKey)
		if e != nil {
			_ = os.RemoveAll(stage)
			fail(w, 400, fmt.Errorf("%s: %w", f.Name, e))
			return
		}
		if sourceFormat != kh3.FormatSteam {
			_ = os.RemoveAll(stage)
			fail(w, 400, fmt.Errorf("%s no se reconoce como guardado cifrado de Epic", f.Name))
			return
		}
		afterOpen, e := sourceBytes(f)
		if e != nil || hashBytes(afterOpen) != f.Hash {
			_ = os.RemoveAll(stage)
			fail(w, http.StatusConflict, fmt.Errorf("%s cambió durante la validación; vuelve a revisar", f.Name))
			return
		}
		plain = kh3.PadToBlock(plain)
		out, e := kh3.Seal(plain, kh3.FormatSteam, key)
		if e != nil {
			_ = os.RemoveAll(stage)
			fail(w, 500, e)
			return
		}
		check, format, e := kh3.Open(out, key)
		if e != nil || format != kh3.FormatSteam {
			_ = os.RemoveAll(stage)
			if e == nil {
				e = errors.New("la salida no se reconoce como guardado Steam")
			}
			fail(w, 500, fmt.Errorf("validación de %s: %w", f.Name, e))
			return
		}
		if len(check) != len(plain) || string(check[:0x0C]) != string(plain[:0x0C]) || string(check[0x10:]) != string(plain[0x10:]) {
			_ = os.RemoveAll(stage)
			fail(w, 500, fmt.Errorf("la relectura de %s no coincide con el original", f.Name))
			return
		}
		if e = os.WriteFile(filepath.Join(newDir, f.Name), out, 0o600); e != nil {
			_ = os.RemoveAll(stage)
			fail(w, 500, e)
			return
		}
	}
	id := randomHex(16)
	item := &prepared{Destination: dest, Source: req.Source, UploadID: req.UploadID, Account: account, Files: files, Stage: stage, Replace: replace, SteamHashes: steamHashes, CreatedAt: time.Now()}
	s.mu.Lock()
	if s.pending == nil {
		s.pending = map[string]*prepared{}
	}
	s.pending[id] = item
	s.mu.Unlock()
	jsonOut(w, 200, map[string]any{"id": id, "files": names(files), "destination": dest, "account": account})
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string `json:"id"`
		Confirm bool   `json:"confirm"`
	}
	if err := decode(w, r, &req); err != nil {
		fail(w, 400, err)
		return
	}
	if !req.Confirm {
		fail(w, 400, errors.New("confirma la instalación para continuar"))
		return
	}
	s.mu.Lock()
	p := s.pending[req.ID]
	delete(s.pending, req.ID)
	s.mu.Unlock()
	if p == nil {
		fail(w, 404, errors.New("la preparación expiró; revisa y convierte otra vez"))
		return
	}
	defer os.RemoveAll(p.Stage)
	if time.Since(p.CreatedAt) > 20*time.Minute {
		fail(w, 410, errors.New("la preparación expiró; vuelve a revisar"))
		return
	}
	// Re-check both sides before making the visible backup, so no file changed
	// between review/preparation and installation.
	for i, f := range p.Files {
		b, err := sourceBytes(f)
		if err != nil || hashBytes(b) != f.Hash {
			fail(w, 409, fmt.Errorf("el archivo Epic %s cambió o ya no está disponible; no se instaló nada", f.Name))
			return
		}
		target := filepath.Join(p.Destination, f.Name)
		_, err = os.Stat(target)
		nowExists := err == nil
		if (err != nil && !errors.Is(err, os.ErrNotExist)) || nowExists != p.Replace[i] {
			fail(w, 409, fmt.Errorf("el destino %s cambió desde la preparación; no se instaló nada", f.Name))
			return
		}
		if nowExists {
			h, e := fileHash(target)
			if e != nil || h != p.SteamHashes[i] {
				fail(w, 409, fmt.Errorf("el archivo de Steam %s cambió desde la preparación; no se instaló nada", f.Name))
				return
			}
		}
	}
	stamp := time.Now().Format("20060102-150405")
	backupRoot := filepath.Join(p.Destination, "Copias de seguridad", "KH3 Epic a Steam "+stamp)
	epicBackup := filepath.Join(backupRoot, "Epic")
	if err := os.MkdirAll(epicBackup, 0o755); err != nil {
		fail(w, 500, err)
		return
	}
	steamBackup := filepath.Join(backupRoot, "Steam reemplazado")
	for i, f := range p.Files {
		if err := writeSourceBackup(f, filepath.Join(epicBackup, f.Name)); err != nil {
			_ = os.RemoveAll(backupRoot)
			fail(w, 500, fmt.Errorf("no se pudo respaldar Epic; no se instaló nada: %w", err))
			return
		}
		if h, err := fileHash(filepath.Join(epicBackup, f.Name)); err != nil || h != f.Hash {
			_ = os.RemoveAll(backupRoot)
			fail(w, 409, fmt.Errorf("la copia de seguridad de Epic %s no coincide; no se instaló nada", f.Name))
			return
		}
		if p.Replace[i] {
			if err := os.MkdirAll(steamBackup, 0o755); err != nil {
				_ = os.RemoveAll(backupRoot)
				fail(w, 500, err)
				return
			}
			if err := copyFile(filepath.Join(p.Destination, f.Name), filepath.Join(steamBackup, f.Name)); err != nil {
				_ = os.RemoveAll(backupRoot)
				fail(w, 500, fmt.Errorf("no se pudo respaldar Steam; no se instaló nada: %w", err))
				return
			}
			if h, err := fileHash(filepath.Join(steamBackup, f.Name)); err != nil || h != p.SteamHashes[i] {
				_ = os.RemoveAll(backupRoot)
				fail(w, 409, fmt.Errorf("la copia de seguridad de Steam %s no coincide; no se instaló nada", f.Name))
				return
			}
		}
	}
	oldDir := filepath.Join(p.Stage, "Steam anterior")
	if err := os.Mkdir(oldDir, 0o700); err != nil {
		_ = os.RemoveAll(backupRoot)
		fail(w, 500, err)
		return
	}
	type moved struct {
		target, old string
		hadOld      bool
	}
	var done []moved
	rollback := func() error {
		var rollbackErrors []error
		for i := len(done) - 1; i >= 0; i-- {
			it := done[i]
			if _, err := os.Stat(it.target); err == nil {
				if err := os.Remove(it.target); err != nil {
					rollbackErrors = append(rollbackErrors, err)
				}
			}
			if it.hadOld {
				if err := os.Rename(it.old, it.target); err != nil {
					rollbackErrors = append(rollbackErrors, err)
				}
			}
		}
		return errors.Join(rollbackErrors...)
	}
	newDir := filepath.Join(p.Stage, "Archivos convertidos")
	for i, f := range p.Files {
		target := filepath.Join(p.Destination, f.Name)
		item := moved{target: target}
		if p.Replace[i] {
			item.old = filepath.Join(oldDir, f.Name)
			if err := os.Rename(target, item.old); err != nil {
				rbErr := rollback()
				fail(w, 500, fmt.Errorf("no se pudo apartar %s: %w%s", f.Name, err, rollbackMessage(rbErr, backupRoot)))
				return
			}
			item.hadOld = true
		}
		done = append(done, item)
		if err := os.Rename(filepath.Join(newDir, f.Name), target); err != nil {
			rbErr := rollback()
			fail(w, 500, fmt.Errorf("no se pudo instalar %s: %w%s", f.Name, err, rollbackMessage(rbErr, backupRoot)))
			return
		}
	}
	jsonOut(w, 200, map[string]any{"destination": p.Destination, "backup": backupRoot, "files": names(p.Files)})
	if p.UploadID != "" {
		s.mu.Lock()
		delete(s.uploads, p.UploadID)
		s.mu.Unlock()
	}
}

func rollbackMessage(err error, backup string) string {
	if err != nil {
		return "; la reversión automática no se completó: " + err.Error() + ". Revisa la copia de seguridad en " + backup
	}
	return "; la instalación parcial se revirtió. Copia de seguridad: " + backup
}

func sourceBytes(f saveFile) ([]byte, error) {
	if f.Path != "" {
		return os.ReadFile(f.Path)
	}
	if len(f.Data) == 0 {
		return nil, errors.New("no hay datos cargados para este archivo")
	}
	return f.Data, nil
}

func writeSourceBackup(f saveFile, dst string) error {
	if f.Path != "" {
		return copyFile(f.Path, dst)
	}
	return os.WriteFile(dst, f.Data, 0o600)
}

func names(files []saveFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Name
	}
	return out
}
func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func fileHash(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return hashBytes(b), nil
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, in)
	closeErr := out.Close()
	if cpErr != nil {
		_ = os.Remove(dst)
		return cpErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func scanEpic(root string) ([]saveFile, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("elige la carpeta de guardados de Epic")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("no se puede abrir la carpeta Epic: %w", err)
	}
	if !st.IsDir() {
		return nil, errors.New("la ruta de Epic debe ser una carpeta")
	}
	baseDepth := pathDepth(abs)
	found := make([]saveFile, 0)
	byName := map[string]string{}
	epicKey, err := kh3.DeriveKey(kh3.EpicAccount)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && pathDepth(p)-baseDepth > 10 {
			return filepath.SkipDir
		}
		if d.IsDir() || !slotName.MatchString(d.Name()) {
			return nil
		}
		// A user may choose the common KINGDOM HEARTS III parent, which can
		// contain both PC platforms. Steam slots have the same filenames, so
		// ignore that branch and require the Epic key for every included file.
		if hasPathComponent(p, "Steam") {
			return nil
		}
		blob, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		_, format, e := kh3.Open(blob, epicKey)
		if e != nil {
			return fmt.Errorf("%s no es un guardado Epic KH III válido (%w); no se convertirá ningún archivo", d.Name(), e)
		}
		if format != kh3.FormatSteam {
			return fmt.Errorf("%s no está cifrado con el contenedor de Epic; consola, KH 1.5 + 2.5 y formatos desconocidos no son compatibles", d.Name())
		}
		if previous := byName[d.Name()]; previous != "" {
			return fmt.Errorf("se encontró más de una copia de %s (%s y %s); elige la carpeta de guardados Epic concreta para no mezclar espacios", d.Name(), previous, p)
		}
		byName[d.Name()] = p
		found = append(found, saveFile{Path: p, Name: d.Name(), Size: int64(len(blob)), Hash: hashBytes(blob), Data: blob})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, errors.New("no se encontraron KHIII_slot*.bin ni KHIII_system.bin válidos de Epic. Elige la carpeta SaveGames/kh3sv2/data o una carpeta superior")
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

func hasPathComponent(p, want string) bool {
	for _, part := range splitPath(p) {
		if strings.EqualFold(part, want) {
			return true
		}
	}
	return false
}
func pathDepth(p string) int { return strings.Count(filepath.Clean(p), string(filepath.Separator)) }

func resolveSteamDestination(folder, account string) (string, string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", "", errors.New("elige una carpeta de destino de Steam")
	}
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", "", err
	}
	if fi, e := os.Stat(abs); e == nil && !fi.IsDir() {
		return "", "", errors.New("el destino de Steam debe ser una carpeta")
	}
	parts := splitPath(abs)
	if account == "" {
		// The ID is reliable only when it is the account folder adjacent to
		// Steam/SaveGames. A numeric Steam accountid from a VDF is not used.
		for i := 1; i < len(parts); i++ {
			if strings.EqualFold(parts[i-1], "Steam") && steamID.MatchString(parts[i]) {
				account = parts[i]
				break
			}
		}
		if account == "" {
			for i := 0; i+1 < len(parts); i++ {
				if steamID.MatchString(parts[i]) && strings.EqualFold(parts[i+1], "SaveGames") {
					account = parts[i]
					break
				}
			}
		}
		if account == "" && strings.EqualFold(filepath.Base(filepath.Clean(abs)), "Steam") {
			entries, readErr := os.ReadDir(abs)
			if readErr == nil {
				var candidates []string
				for _, entry := range entries {
					if entry.IsDir() && steamID.MatchString(entry.Name()) {
						candidates = append(candidates, entry.Name())
					}
				}
				if len(candidates) == 1 {
					account = candidates[0]
				}
			}
		}
	}
	if !steamID.MatchString(account) {
		return "", "", errors.New("no pude detectar el SteamID64 desde la carpeta. Es el número de 17 dígitos de Steam/<SteamID64>/SaveGames; no es el accountid de steam_autocloud.vdf. Escríbelo sin espacios")
	}
	dest := abs
	base := strings.ToLower(filepath.Base(filepath.Clean(dest)))
	if base == "steam" {
		dest = filepath.Join(dest, account, "SaveGames", "kh3sv2", "data")
	} else if base == account {
		dest = filepath.Join(dest, "SaveGames", "kh3sv2", "data")
	} else if base == "savegames" {
		dest = filepath.Join(dest, "kh3sv2", "data")
	} else if base == "kh3sv2" {
		dest = filepath.Join(dest, "data")
	}
	return dest, account, nil
}
func splitPath(p string) []string {
	return strings.FieldsFunc(filepath.Clean(p), func(r rune) bool { return r == '/' || r == '\\' })
}

func pickFolder(prompt string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("el selector integrado está disponible en Windows; escribe o pega la ruta de la carpeta")
	}
	// The prompt is selected from fixed strings above, never supplied by HTTP.
	script := "$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='" + prompt + "'; $d.ShowNewFolderButton=$false; if($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($d.SelectedPath)}"
	out, err := exec.Command("powershell", "-NoProfile", "-STA", "-Command", script).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", errors.New("selección cancelada")
		}
		return "", fmt.Errorf("no se pudo abrir el selector de carpetas: %w", err)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", errors.New("selección cancelada")
	}
	return p, nil
}

func Serve() error {
	token := randomHex(32)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	addr := ln.Addr().String()
	s := &Server{token: token, addr: addr, mux: http.NewServeMux(), pending: map[string]*prepared{}, uploads: map[string][]saveFile{}}
	s.mux.HandleFunc("/", s.guard(s.index))
	s.mux.HandleFunc("/api/discover", s.guard(s.discover))
	s.mux.HandleFunc("/api/browse", s.guard(s.browse))
	s.mux.HandleFunc("/api/upload", s.guard(s.upload))
	s.mux.HandleFunc("/api/release", s.guard(s.releaseUpload))
	s.mux.HandleFunc("/api/review", s.guard(s.review))
	s.mux.HandleFunc("/api/prepare", s.guard(s.prepare))
	s.mux.HandleFunc("/api/install", s.guard(s.install))
	url := "http://" + addr + "/?t=" + token
	if err := exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start(); err != nil {
		fmt.Printf("Abre esta dirección en tu navegador: %s\n", url)
	}
	fmt.Println("Asistente KH III Epic → Steam en 127.0.0.1. No se conecta a internet.")
	return http.Serve(ln, s.mux)
}

// ParseSteamID is exported for focused validation tests and UI consumers.
func ParseSteamID(id string) bool {
	return steamID.MatchString(id) && func() bool { _, e := strconv.ParseUint(id, 10, 64); return e == nil }()
}
