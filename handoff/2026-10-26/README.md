# 원격 복구 자료 — 2026-10-26

이 브랜치는 이어받기용 기록이다. 미완성 구현을 dev/main에 통합하거나 기존 검사·승인·진단 횟수 제한을 해제하지 않는다. 현재 작업 지침은 최신 `origin/dev:HANDOFF.md`를 읽는다.

## 새 환경에서 복구

```bash
git clone https://github.com/cskwork/scopeweaver.git
cd scopeweaver
git fetch origin
git switch dev
git worktree add -b fix/pr40-resume ../scopeweaver-pr40-resume \
  origin/checkpoint/pr40-resume-20261026
git worktree add -b fix/pr46-resume ../scopeweaver-pr46-resume \
  origin/checkpoint/pr46-resume-20261026
git worktree add --detach ../scopeweaver-handoff-evidence \
  origin/checkpoint/handoff-evidence-20261026
python3 ../scopeweaver-handoff-evidence/handoff/2026-10-26/verify.py \
  --extract ../scopeweaver-resume-evidence
```

같은 로컬 브랜치/디렉터리가 이미 있다면 새 이름을 사용한다. 기존 작업을 reset하거나 덮어쓰지 않는다. shallow/single-branch clone이면 checkpoint refs를 별도로 fetch해야 한다. 위 절차는 외부 모델·provider, DB, 앱, 브라우저나 과거 테스트를 실행하지 않는다.

`RECOVERY.json`은 소스 체크포인트 SHA와 원래 미커밋 7개 파일의 SHA256을 고정한다. #40은 이 7개 파일까지 커밋된 상태이므로 이전 로컬 patch나 bundle을 따로 적용하면 안 된다. #49 코드는 기존 `origin/feat/gh-49-opencode-sdk`에 있으며, #50/#52/#55/#56은 dev에 통합됐다.

## 증거 경로

`EVIDENCE.tar.gz` 안의 디렉터리별 원본 위치와 모든 파일 목록은 `MANIFEST.json`의 `roots`/`files`에 있다.

| 압축 내 디렉터리 | 내용 |
| --- | --- |
| `pr40/` | PLAN, 인계/승인/번역 입력 목록, 원본 RED/GREEN, SDK/호환성/문서 리뷰, 재현 스크립트와 텍스트 기록 |
| `pr46/` | CI 수정 계획·리뷰·검증 기록 |
| `gh49-plan/`, `gh49-evidence/` | OpenCode SDK 계획, 감사/호환성 기록, 진단 계획·한도·관찰 결과·provider attribution ledgers |
| `gh15/` | 남아 있는 언어 확장 계획 기록; 구현은 dev 및 PR #52 이력 |
| `gh50/` | 병합된 scope/shutdown 작업 기록 |
| `gh53-verification/`, `gh53-retry/` | compaction attribution·retry-dialog 검증과 재현 자료 |
| `release/` | 전체 통합·main/minor release 조건, 별도 진단 승인·중단 기록 |

과거 파일의 `/Users/danny/...`, `/private/tmp/...` 경로는 당시 호스트 위치다. `roots`와 압축 내 경로를 대응시켜 새 환경에 맞게 바꾼다. 오래된 “미커밋/실행 중/CI 대기” 문구는 역사적 상태이며 `RECOVERY.json`과 최신 dev 인계 문서가 현재 원격 보존 상태에 우선한다. 실제 작업 재개 시 GitHub 상태·새 변경을 다시 확인한다.

## 정확성·민감정보·제외 내역

- `source_sha256`은 로컬 원본을 식별한다. `published_sha256`은 공개용 사본을 식별한다. `redacted_or_normalized=true`인 파일을 원본과 바이트가 같은 증거라고 주장하지 않는다.
- credential-like 값과 signed access URL 값은 `__REDACTED_...__`로 치환했다. 같은 값에는 같은 opaque marker를 사용하며, 비밀값→marker 매핑은 게시하지 않는다. 원래 로컬 파일은 바꾸지 않았다.
- 검증한 앱 소스는 checkpoint 브랜치에 그대로 있다. 증거에 담긴 과거 실행 스크립트는 일부 임시 인증값이 제거됐으므로 새 disposable 자격 증명과 경로를 설정한 뒤 검토해서 사용한다.
- DB 파일/WAL, 캐시·설치된 의존성, 재생성 가능한 바이너리, 개인 인증 설정, 검토하지 않은 화면 이미지·컨테이너 파일은 포함하지 않았다. 포함하지 않은 파일도 manifest에 파일별 사유가 있다. 추출된 텍스트 ledger와 기록은 가능한 범위에서 포함했다.
- 이미지/바이너리를 제외한 사실을 성공한 UI 검증의 새로운 증거로 바꾸지 않는다. 원래 GitHub artifact의 ID·해시·기록과 로컬 원본의 존재 여부를 먼저 확인한다. 이번 저장 작업은 이미 끝난 다운로드·실행을 반복하지 않았다.
- 이 패키지에 없는 파일이나 당시 이미 사라진 임시 파일이 필요하면 그 공백을 명시한다. 원본 실행이 재현됐다고 만들어 내지 않는다.

## 계속 남는 제한

#40의 거부된 planner/worker 번역 생성 재시도·provider 우회와 검사 완화는 허용되지 않는다. #57의 hosted 진단은 1/1 사용 후 중단됐으며 기존 3회 시도 및 비활성 Phase B 한도를 초기화하지 않는다. SDK 감사 실패·과거 원인 미확정·최종 통합 검증·maintainer 확인은 별도 조건이다. 보관 완료는 병합 또는 릴리스 준비 완료가 아니다.
