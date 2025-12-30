package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	addr         = ":8080"
	uploadDir    = "./uploads"
	maxUploadMB  = 20   // 파일 최대 크기 제한
	maxFileCount = 1000 // 최대 파일 개수 제한
)

var allowedExt = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".pdf":  true,
	".txt":  true,
}

var allowedMimePrefix = []string{
	"image/",
	"application/pdf",
	"text/plain",
	"text/plain;",
}

type UploadResp struct {
	OK        bool   `json:"ok"`
	Token     string `json:"token,omitempty"`
	ShareURL  string `json:"share_url,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Error     string `json:"error,omitempty"`
}

func main() {
	if err := os.MkdirAll(uploadDir, 0o700); err != nil {
		panic(err)
	}

	// PORT 환경변수 사용 (Render 배포 대응)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	serverAddr := ":" + port

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/upload", handleUpload)
	mux.HandleFunc("/s/", handleShareDownload)

	srv := &http.Server{
		Addr:              serverAddr,
		Handler:           logMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	fmt.Println("Listening on", serverAddr)
	if err := srv.ListenAndServe(); err != nil {
		panic(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>File Upload</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      max-width: 800px;
      margin: 50px auto;
      padding: 20px;
      background: #f5f5f5;
    }
    .container {
      background: white;
      padding: 30px;
      border-radius: 8px;
      box-shadow: 0 2px 8px rgba(0,0,0,0.1);
    }
    h1 {
      margin: 0 0 20px 0;
      color: #333;
      font-size: 24px;
    }
    form {
      margin-bottom: 20px;
    }
    input[type="file"] {
      display: block;
      width: 100%;
      padding: 10px;
      margin-bottom: 10px;
      border: 2px solid #ddd;
      border-radius: 4px;
      box-sizing: border-box;
    }
    button {
      background: #007bff;
      color: white;
      border: none;
      padding: 10px 20px;
      border-radius: 4px;
      cursor: pointer;
      font-size: 14px;
    }
    button:hover {
      background: #0056b3;
    }
    .info {
      background: #f8f9fa;
      padding: 15px;
      border-radius: 4px;
      font-size: 14px;
      color: #666;
    }
    .info p {
      margin: 5px 0;
    }
    code {
      background: #e9ecef;
      padding: 2px 6px;
      border-radius: 3px;
      font-size: 13px;
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>File Upload</h1>
    <form action="/upload" method="post" enctype="multipart/form-data">
      <input type="file" name="file" required />
      <button type="submit">Upload</button>
    </form>
    <div class="info">
      <p><strong>업로드 가능한 확장자:</strong></p>
      <p><code>.png</code> <code>.jpg</code> <code>.jpeg</code> <code>.gif</code> <code>.pdf</code> <code>.txt</code></p>
      <p style="margin-top:15px"><strong>curl 업로드 예시:</strong></p>
      <p><code>curl -F "file=@yourfile.png" https://file-share-go.onrender.com/upload</code></p>
      <p style="margin-top:15px"><strong>curl 다운로드 예시:</strong></p>
      <p><code>curl -OJ https://file-share-go.onrender.com/s/TOKEN</code></p>
      <p style="margin-top:5px; font-size:12px; color:#999">* -O: 파일로 저장, -J: 원본 파일명 사용</p>
    </div>
  </div>
</body>
</html>`)
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 업로드된 파일 개수 체크 (DoS 방지)
	if err := checkFileCount(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, UploadResp{OK: false, Error: err.Error()})
		return
	}

	// 최대 업로드 크기 제한 (DoS 방지)
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxUploadMB)<<20)

	// multipart 파싱
	if err := r.ParseMultipartForm(int64(maxUploadMB) << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Error: "invalid multipart or too large"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Error: "file is required"})
		return
	}
	defer file.Close()

	savedName, size, err := validateAndSave(file, header)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, UploadResp{OK: false, Error: err.Error()})
		return
	}

	// 공유 토큰 생성
	token := randToken(16)

	origName := header.Filename
	metadata := fmt.Sprintf("%s|%s", savedName, origName)
	if err := os.WriteFile(filepath.Join(uploadDir, token+".meta"), []byte(metadata), 0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, UploadResp{OK: false, Error: "failed to save metadata"})
		return
	}

	shareURL := fmt.Sprintf("%s://%s/s/%s", schemeOf(r), r.Host, token)

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Upload Success</title>
  <style>
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      max-width: 800px;
      margin: 50px auto;
      padding: 20px;
      background: #f5f5f5;
    }
    .container {
      background: white;
      padding: 30px;
      border-radius: 8px;
      box-shadow: 0 2px 8px rgba(0,0,0,0.1);
      text-align: center;
    }
    h1 {
      color: #28a745;
      margin-bottom: 20px;
      font-size: 24px;
    }
    .share-box {
      background: #f8f9fa;
      padding: 15px;
      border-radius: 4px;
      margin: 20px 0;
    }
    .share-box p {
      margin: 5px 0 10px 0;
      color: #666;
      font-size: 14px;
    }
    .share-link {
      display: block;
      color: #007bff;
      word-break: break-all;
      text-decoration: none;
      padding: 10px;
      background: white;
      border-radius: 4px;
      border: 1px solid #ddd;
    }
    .share-link:hover {
      background: #e9ecef;
    }
    .back-btn {
      display: inline-block;
      background: #007bff;
      color: white;
      text-decoration: none;
      padding: 10px 20px;
      border-radius: 4px;
      margin-top: 10px;
    }
    .back-btn:hover {
      background: #0056b3;
    }
  </style>
</head>
<body>
  <div class="container">
    <h1>✓ Upload Successful!</h1>
    <div class="share-box">
      <p><strong>공유 링크:</strong></p>
      <a href="%s" class="share-link">%s</a>
    </div>
    <a href="/" class="back-btn">← 돌아가기</a>
  </div>
</body>
</html>`, shareURL, shareURL)
		return
	}

	writeJSON(w, http.StatusOK, UploadResp{
		OK:        true,
		Token:     token,
		ShareURL:  shareURL,
		FileName:  savedName,
		SizeBytes: size,
	})
}

func checkFileCount() error {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return errors.New("failed to check storage")
	}

	// .meta 파일을 제외한 실제 업로드 파일만 카운트
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".meta") {
			count++
		}
	}

	if count >= maxFileCount {
		return fmt.Errorf("storage full (max %d files)", maxFileCount)
	}
	return nil
}

func handleShareDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimPrefix(r.URL.Path, "/s/")
	token = strings.TrimSpace(token)
	if token == "" || !isSafeToken(token) {
		http.Error(w, "invalid token", http.StatusBadRequest)
		return
	}

	metaPath := filepath.Join(uploadDir, token+".meta")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// 메타데이터 파싱 (저장된파일명|원본파일명)
	parts := strings.SplitN(strings.TrimSpace(string(b)), "|", 2)
	savedName := parts[0]
	origName := "download"
	if len(parts) == 2 {
		origName = sanitizeFilename(parts[1])
	}

	// 저장된 파일명도 안전한지 재검증(방어적)
	if savedName == "" || strings.Contains(savedName, "..") || strings.ContainsAny(savedName, `/\`) {
		http.Error(w, "invalid metadata", http.StatusInternalServerError)
		return
	}

	fullPath := filepath.Join(uploadDir, savedName)

	// 파일 존재 확인
	f, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	// MIME 다시 감지해서 Content-Type 설정
	head := make([]byte, 512)
	n, _ := f.Read(head)
	_, _ = f.Seek(0, 0)
	mime := http.DetectContentType(head[:n])

	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, origName))

	http.ServeContent(w, r, origName, time.Now(), f)
}

func validateAndSave(file multipart.File, header *multipart.FileHeader) (string, int64, error) {
	origName := header.Filename
	origName = strings.ReplaceAll(origName, "\x00", "")
	base := filepath.Base(origName) // 경로 제거
	ext := strings.ToLower(filepath.Ext(base))

	if !allowedExt[ext] {
		return "", 0, fmt.Errorf("extension not allowed: %s", ext)
	}

	// MIME sniffing (클라이언트가 준 Content-Type 믿지 않음)
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", 0, errors.New("failed to read file header")
	}
	mime := http.DetectContentType(buf[:n])

	if !isAllowedMime(mime) {
		return "", 0, fmt.Errorf("mime not allowed: %s", mime)
	}

	// 저장 파일명은 랜덤 + 확장자
	saveName := randToken(12) + ext
	dstPath := filepath.Join(uploadDir, saveName)

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, errors.New("failed to create file")
	}
	defer dst.Close()

	written1, err := dst.Write(buf[:n])
	if err != nil {
		return "", 0, errors.New("failed to write file")
	}
	written2, err := io.Copy(dst, file)
	if err != nil {
		return "", 0, errors.New("failed to save file")
	}
	return saveName, int64(written1) + written2, nil
}

func isAllowedMime(m string) bool {
	for _, p := range allowedMimePrefix {
		// "/"나 ";"로 끝나면 prefix 매칭
		if strings.HasSuffix(p, "/") || strings.HasSuffix(p, ";") {
			if strings.HasPrefix(m, p) {
				return true
			}
		} else {
			// 정확히 일치 검사
			if m == p {
				return true
			}
		}
	}
	return false
}

func randToken(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isSafeToken(s string) bool {
	// hex 토큰만 허용
	if len(s) < 10 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func sanitizeFilename(name string) string {
	var result strings.Builder
	result.Grow(len(name))

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			(r >= '가' && r <= '힣') ||
			r == ' ' || r == '-' || r == '_' || r == '.' {
			result.WriteRune(r)
		}
	}
	name = result.String()

	for strings.Contains(name, "..") {
		name = strings.ReplaceAll(name, "..", ".")
	}

	name = filepath.Base(name)

	if len(name) > 255 {
		ext := filepath.Ext(name)
		nameWithoutExt := strings.TrimSuffix(name, ext)

		// 바이트 길이가 200을 넘으면 rune 단위로 안전하게 자르기
		if len(nameWithoutExt) > 200 {
			runes := []rune(nameWithoutExt)
			for len(string(runes)) > 200 && len(runes) > 0 {
				runes = runes[:len(runes)-1]
			}
			nameWithoutExt = string(runes)
		}
		name = nameWithoutExt + ext
	}

	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		name = "download"
	}

	return name
}

func schemeOf(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return "https"
	}
	return "http"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("%s %s %s\n", r.Method, r.URL.Path, time.Since(start))
	})
}
