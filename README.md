# File Upload & Share Service

Go로 구현한 안전한 파일 업로드 및 공유 서비스입니다.

🌐 **Live Demo**: https://file-share-go.onrender.com/

## 주요 기능

- 웹 브라우저와 curl을 통한 파일 업로드
- 토큰 기반 공유 링크 생성
- 보안 검증 (MIME 타입, 확장자, 경로 탐색 방지)
- 파일 크기 및 개수 제한

## 지원하는 파일 형식

- 이미지: `.png`, `.jpg`, `.jpeg`, `.gif`
- 문서: `.pdf`, `.txt`

## 보안 기능

1. **MIME 타입 검증**: 클라이언트가 보낸 Content-Type을 신뢰하지 않고 실제 파일 내용을 분석
2. **확장자 화이트리스트**: 허용된 확장자만 업로드 가능
3. **경로 탐색 공격 방지**: `..`, `/`, `\` 등 위험한 문자 필터링
4. **파일 크기 제한**: 최대 20MB
5. **파일 개수 제한**: 최대 1000개 (디스크 고갈 방지)
6. **안전한 파일명**: 랜덤 토큰으로 파일명 생성
7. **안전한 파일 권한**: `0o600` (소유자만 읽기/쓰기)

## 로컬 실행 방법

```bash
# 저장소 클론
git clone https://github.com/chh41/file_upload.git
cd file_upload

# 실행
go run main.go

# 브라우저에서 접속
# http://localhost:8080
```

## 사용 방법

### 웹 브라우저로 업로드
1. https://file-share-go.onrender.com/ 접속
2. 파일 선택 후 Upload 버튼 클릭
3. 생성된 공유 링크로 다운로드

### curl로 업로드
```bash
curl -F "file=@yourfile.png" https://file-share-go.onrender.com/upload
```

응답 예시:
```json
{
  "ok": true,
  "token": "f9404fb7753cf41c8ee4dce4b5f7c2fc",
  "share_url": "https://file-share-go.onrender.com/s/f9404fb7753cf41c8ee4dce4b5f7c2fc",
  "file_name": "3f632dc0953d3ba217fbc054.png",
  "size_bytes": 67
}
```

### curl로 다운로드
```bash
curl https://file-share-go.onrender.com/s/{토큰} -o downloaded.png
```

## 기술 스택

- **백엔드**: Go
- **프론트엔드**: HTML/CSS
- **배포**: Render (Docker)

## 프로젝트 구조

```
file_upload/
├── main.go          # 메인 애플리케이션 (백엔드 + 프론트엔드 HTML 포함)
├── uploads/         # 업로드된 파일 저장소
│   └── .gitkeep     # 빈 폴더 유지용
├── Dockerfile       # Docker 컨테이너 설정
├── .gitignore       # Git 제외 파일 목록
├── go.mod           # Go 모듈 정의
└── README.md        # 프로젝트 문서
```

## 환경 변수

- `PORT`: 서버 포트 (기본값: 8080)

## 제한사항

- 최대 파일 크기: 20MB
- 최대 파일 개수: 1000개
- 지원 파일 형식: png, jpg, jpeg, gif, pdf, txt
