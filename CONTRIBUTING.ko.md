# ScopeWeaver 기여 안내

[English](CONTRIBUTING.md)

버그 수정, ARTEX 원본 백포트, 의존성 업그레이드, 문서, 테스트 등 모든 기여를 환영합니다.

## 시작하기 전에

- [사용 제한](README.ko.md#라이선스와-면책-조항)을 읽어 주세요. 실제 온라인 시스템을 대상으로 하는
  기능을 추가하거나 사용 제한을 약화하는 기여는 받지 않습니다.
- 큰 변경은 먼저 이슈를 열어 방향을 합의해 주세요.
- PR 하나에는 한 가지 주제만 담아 주세요. 작은 PR일수록 빨리 리뷰됩니다.

## PR 요구 사항

- **영어와 한국어를 함께 반영합니다.** 사용자에게 보이는 문구는 두 언어가 모두 필요합니다.
  - 서버 메시지: `locale/`의 `locale.Register(key, en, ko)`
  - 웹 UI: `web/src/i18n/messages/`의 `en`, `ko` 객체
  - 문서: `README.md`와 `README.ko.md`, `docs/`와 `docs/ko/`
- **바뀐 동작에는 테스트를 추가합니다.** 변경이 없으면 실패하는 테스트를 추가하거나 고쳐 주세요.
  기존 검사를 끄거나, 건너뛰거나, 느슨하게 만들어 통과시키지 마세요.
- **일회용 데이터베이스만 사용합니다.** PostgreSQL에 쓰는 테스트는 `scopeweaver_test_*`
  (에이전트 어댑터 E2E는 `scopeweaver_adapter_*`) 이름의 새 데이터베이스를 사용해야 합니다.
- **원본 작업을 표기합니다.** ARTEX 백포트에는 `Upstream-commit: mhtsec/ARTEX@<sha>`와 원저자의
  `Co-authored-by:` 트레일러를 추가하고, `docs/PROVENANCE.md`와 `docs/ko/PROVENANCE.md`를
  갱신해 주세요.
- **라이선스.** 기여는 저장소의 [AGPL-3.0](LICENSE) 라이선스로 받습니다.

## 실행할 검사

[.github/workflows/ci.yml](.github/workflows/ci.yml)의 CI 작업과 같습니다.

```bash
# 백엔드: 모든 Go 패키지 (ARTEX_PG_DSN을 일회용 scopeweaver_test_* 데이터베이스로 지정)
go test ./... -count=1

# 프런트엔드
cd web && npm ci && npm test && npm run build:static

# 에이전트 어댑터 (scopeweaver_adapter_* 데이터베이스를 사용하는 실제 백엔드 E2E 포함)
cd adapters/agent && npm ci && npm test && npm run check
SCOPEWEAVER_ADAPTER_E2E=1 go test ./server -run '^TestAgentAdapterE2E$' -count=1
```

`e2e` 워크플로는 모든 PR과 머지 후에 실제 바이너리를 대상으로 핵심 브라우저 스모크 테스트도 실행합니다.
실행 중인 서버와 새 `scopeweaver_test_*` 데이터베이스로 로컬에서 실행하려면
`psql`과 같은 체크아웃에서 빌드한 임베디드 바이너리가 필요합니다.

```bash
cd e2e && npm ci && npx playwright install chromium
SCOPEWEAVER_URL=http://127.0.0.1:8787 ARTEX_PG_DSN=postgres://.../scopeweaver_test_e2e \
  SCOPEWEAVER_BINARY=/absolute/path/to/scopeweaver npm run smoke
```

기존 스모크 검사에서 비밀번호를 초기화한 뒤, 대시보드 회귀 검사는 같은 시험 DB를 사용하는
격리된 바이너리 복사본을 빈 루프백 포트에서 실행합니다. 영어·한국어의 기존·신규 토큰 누적과
프로필 필터를 검사하고, 시험 서버를 종료한 뒤 자신이 만든 데이터를 삭제합니다. 대시보드 증적과
시험 서버 로그는 `e2e/e2e-results/dashboard-token-chart/`에 저장됩니다.

PR 설명에 실행한 검사와 실행하지 못한 검사를 적어 주세요.

## 리뷰와 머지

1. `frontend`, `agent-adapter`, `backend` 검사와 `e2e` 스모크 테스트가 모두 통과해야 합니다. 처음
   기여하는 경우 관리자가 CI 실행을 승인합니다.
2. PR은 diff를 기준으로 정확성, 보안, 영어·한국어 일치 여부를 리뷰합니다.
3. **모든 PR은 머지 전에 관리자가 직접 최종 확인합니다.** 이 확인 없이는 머지하지 않습니다.

릴리스는 머지 전후로 전체 E2E 테스트를 거친 뒤 관리자가 진행합니다.
