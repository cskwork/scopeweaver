# LLM 제공자 템플릿

ScopeWeaver는 **Anthropic**, **OpenAI Chat Completions**, **OpenAI Responses**, 기본 **Ollama**의 네 가지 프로필 형식을 지원합니다. 모든 형식에서 사용자 지정 기본 URL, 프록시, 세션 헤더, 재시도, 속도 제한, 동시 요청 설정을 그대로 사용할 수 있으므로 클라우드 제공자, 게이트웨이, 직접 운영하는 OpenAI 호환 서버도 이전처럼 동작합니다. Ollama는 선택 사항이며 클라우드 제공자는 기본적으로 계속 호환됩니다.

템플릿은 자격 증명을 저장하거나 프로필을 활성화하거나 기존 프로필을 변경하지 않고 **새 프로필의 양식을 채웁니다**. Ollama 템플릿은 선택한 서버에 설치된 모델도 조회하며, 클라우드 템플릿은 요청을 보내지 않습니다. 템플릿을 적용한 뒤에도 형식, 기본 URL, 모델은 수정할 수 있습니다.

## Z.ai GLM-5.3

![GLM-5.3 새 프로필 템플릿](../../screenshots/ko/llm-templates.png)

API 키를 입력하지 않은 템플릿 화면이며 실제 제공자 연결 결과가 아닙니다.

**LLM → 새로 만들기 → 설정 템플릿**을 여세요. API 과금에는 **일반 API(권장)**를 선택합니다. **Coding Plan(참고용)**은 ScopeWeaver 사용에 대해 Z.ai의 허가를 받은 경우에만 사용하세요.

| 항목 | 일반 API | Coding Plan 참고용 |
| --- | --- | --- |
| 형식 | OpenAI Chat Completions | OpenAI Chat Completions |
| 기본 URL | `https://api.z.ai/api/paas/v4` | `https://api.z.ai/api/coding/paas/v4` |
| 모델 | `glm-5.3` | `glm-5.3` |
| 추론 | `enabled` | `enabled` |
| 추론 강도 | `max` | `max` |
| 문맥 창(K) | `1000` | `1000` |
| 출력 한도 | `0`(생략, 제공자 기본값) | `0`(생략, 제공자 기본값) |

두 엔드포인트의 과금은 구분됩니다. 일반 API 사용량은 Coding Plan 구독 할당량과 별개입니다. [Z.ai 연결 설정](https://zcode.z.ai/en/docs/configuration)을 참고하고, 테스트 전에 계정의 모델 접근 권한과 과금 조건을 확인하세요.

GLM-5.3은 텍스트 입력과 100만 토큰의 문맥 창을 지원하며 추론을 항상 켜야 합니다. 지원 강도는 `low`, `high`, `max`이며 템플릿은 `max`를 사용합니다. 모델 이름은 수정할 수 있으므로 다른 모델을 선택하면 추론과 문맥 설정도 확인하세요. [공식 GLM-5.3 안내](https://docs.z.ai/guides/llm/glm-5.3)를 참고하세요.

**Coding Plan 지원 범위:** Z.ai는 공식 지원 도구로 사용을 제한하며 ScopeWeaver는 목록에 없습니다. 엔드포인트 템플릿이 사용 허가나 공식 지원을 의미하지는 않습니다. 여기서 사용하려면 먼저 Z.ai의 허가를 받으세요. [도구 연동 안내](https://docs.z.ai/devpack/tool/others)와 [이용 정책](https://docs.z.ai/devpack/usage-policy)을 확인하세요. ScopeWeaver는 지원 도구로 위장하거나 과금 엔드포인트를 자동으로 바꾸지 않습니다.

프로필 이름과 본인의 API 키를 입력하세요. 템플릿을 적용해도 기존에 입력한 이름, 키, 프록시, 세션 헤더 및 관련 없는 설정은 유지합니다. 준비되면 저장하고, 새 프로필은 별도로 활성화하세요. 기존 **연결 테스트**와 **모델 불러오기** 버튼을 클릭하면 실제 제공자 요청이 발생합니다. 이번 변경에서는 실제 제공자 테스트를 수행하지 않았습니다.

공식 문서 확인일: **2026-10-03**. 모델 제공 여부, 계정 권한 및 정책은 바뀔 수 있습니다.

## 기본 Ollama(로컬 추론)

ScopeWeaver는 Ollama의 기본 `/api/chat` API를 사용합니다. Ollama의 OpenAI 호환 엔드포인트에는 문맥 크기를 지정하는 방법이 없으므로([Ollama OpenAI 호환성](https://docs.ollama.com/api/openai-compatibility)) `/v1`을 가리키는 프로필은 `num_ctx`를 보장할 수 없습니다. 기본 형식은 요청마다 이 값을 보냅니다. `/v1`, `/api`, `/api/chat`으로 끝나는 기본 URL은 서버 루트로 정규화합니다.

Ollama 공식 문서의 기본 문맥 값은 페이지마다 다릅니다. [FAQ](https://docs.ollama.com/faq)는 4,096 토큰이라고 설명하고, [문맥 길이 페이지](https://docs.ollama.com/context-length)는 VRAM에 따라 달라진다고 설명합니다(24GiB 미만 약 4K, 24–48GiB 32K, 그 이상 256K). 따라서 ScopeWeaver는 서버 기본값에 의존하지 않습니다.

### 템플릿

**LLM → 새로 만들기 → 설정 템플릿 → 로컬 추론(Ollama)**을 여세요.

| 필드 | 같은 컴퓨터 | Docker에서 호스트 접근 | 번들 Compose 서비스 |
| --- | --- | --- | --- |
| 형식 | Ollama(네이티브 /api/chat) | Ollama(네이티브 /api/chat) | Ollama(네이티브 /api/chat) |
| 기본 URL | `http://127.0.0.1:11434` | `http://host.docker.internal:11434` | `http://ollama:11434` |
| 모델 | 이 서버에 설치된 모델 | 이 서버에 설치된 모델 | 이 서버에 설치된 모델 |
| API 키 | 비어 있음 | 비어 있음 | 비어 있음 |
| 문맥 창(K) | `32`(`num_ctx` 32,768) | `32` | `32` |
| 최대 동시 요청 수 | `1` | `1` | `1` |
| 모델 유지 시간 | `30m` | `30m` | `30m` |
| 출력 한도 | `4096` | `4096` | `4096` |
| 추론 / 강도 | 생략(모델 기본값) | 생략 | 생략 |

Ollama 템플릿을 선택하면 해당 엔드포인트의 `/api/tags`와 모델 메타데이터를 읽고, 설치된 모델 중 `tools`를 선언한 모델을 우선 선택합니다. 새 프로필에서 형식을 Ollama로 바꿔도 같은 조회를 합니다. 엔드포인트가 비어 있거나 클라우드 템플릿 주소 그대로이면 호스트 기본 주소를 쓰며, 사용자 지정 주소는 유지합니다. 조회 중에는 모델 칸을 비우고, 실패하거나 목록이 비어 있으면 오류와 함께 빈 상태로 둡니다. 엔드포인트, 선택적 토큰, 프록시를 확인한 뒤 **모델 불러오기**를 누르거나 모델 태그를 직접 입력하세요. 조회 중 모델이나 연결 필드를 수정하면 자동 선택을 취소하며, 늦게 도착한 응답은 수정한 값을 덮어쓰지 않습니다. 기존 프로필을 열 때는 저장된 모델을 유지합니다. 선택만으로 모델을 내려받거나 실행하거나 프로필을 저장·활성화하지 않습니다. **연결 테스트**는 누를 때만 실행합니다.

Ollama 템플릿 선택과 Ollama 형식 전환은 조회 전에 저장하지 않은 키를 지웁니다. **API 키 동작:** Ollama 템플릿은 프로필 이름, 프록시, 세션 헤더 등 입력한 설정을 유지하지만 **저장하지 않은 API 키는 지웁니다**. 클라우드 자격 증명이 로컬 엔드포인트로 전송되지 않게 하기 위해서입니다. GLM 템플릿은 입력한 키를 계속 유지합니다. 로컬 Ollama는 키를 비워 두세요. Ollama 앞의 리버스 프록시가 요구할 때만 토큰을 입력하면 `Authorization: Bearer`로 전송합니다. 키 없는 Ollama 프로필도 테스트, 저장, 활성화, 장애 조치에 사용할 수 있으며, 프로필 카드에는 키 누락 경고 대신 **키 불필요**가 표시됩니다. 저장된 네이티브 Ollama 프로필을 수정할 때 키 칸을 비워 두면 저장된 토큰을 유지하고, 클라우드 프로필을 Ollama로 바꾸면서 비워 두면 기존 클라우드 키를 삭제합니다.

**다른 형식의 키:** Anthropic·OpenAI 형식은 기본 URL이 명시적으로 로컬이 아니면 키가 필요합니다. 명시적 로컬이란 호스트가 루프백 또는 사설 주소로만 확인되는 경우입니다. 따라서 vLLM, LM Studio 같은 키 없는 로컬 OpenAI 호환 서버는 키를 비워 두어도 동작합니다. 이때 ScopeWeaver는 내부의 비밀이 아닌 자리표시 값을 보내 SDK가 환경 변수의 `OPENAI_API_KEY`나 `ANTHROPIC_API_KEY`를 물려받지 않게 합니다. 로컬이 아닌 엔드포인트에 키가 없으면 거부합니다(*llm: 로컬이 아닌 호환 엔드포인트에는 API 키가 필요합니다*).

### 문맥, 출력, 모델 유지 설정

| 설정 | Ollama 동작 | 클라우드 형식 |
| --- | --- | --- |
| 문맥 창(K) | 이진 K로 `options.num_ctx`에 전송(32 = 32,768). `0`이면 32, 16 미만이면 16. 압축 기준에도 사용 | 기존과 같음: 압축 기준만, `0` = 200K |
| 출력 한도 | `options.num_predict`로 전송, `0`이면 4,096 | 기존과 같음: `0`이면 필드 생략 |
| 모델 유지 시간 | `keep_alive`로 전송, 비우면 `30m`(Ollama 자체 기본값은 5분) | 전송하지 않음 |
| 최대 동시 요청 수 | `0`이면 1. 프로필이 여러 개여도 Ollama 서버마다 입장 제어 하나를 공유하며 가장 낮은 설정값을 적용 | `0`이면 **시스템 설정 → 최대 동시 LLM 요청 수**를 따름 |

대기 중인 승인 판정·주 에이전트·곁가지 질문 요청은 백그라운드 작업보다 다음 빈 슬롯을 우선 배정받습니다. 각 그룹 안에서는 먼저 온 순서이며 진행 중인 추론은 중단하지 않습니다. 연결 진단과 예열은 백그라운드 요청입니다.

서버에서 `OLLAMA_NUM_PARALLEL`을 1보다 크게 설정했을 때만 동시 요청 수를 늘리세요. Ollama의 메모리 사용량은 `OLLAMA_NUM_PARALLEL × 문맥 길이`에 비례해 늘어납니다([FAQ](https://docs.ollama.com/faq)).

### 실행 중 보호 장치

- **예열:** 기본 프로필로 첫 추론을 하기 전에 `/api/show`를 호출하고, `tools`를 선언하지 않은 모델은 거부합니다. 그다음 프로필의 `num_ctx`와 `keep_alive`로 빈 `/api/chat` 요청을 보내 모델을 불러옵니다. 예열도 같은 동시 요청 슬롯 안에서 실행합니다.
- **압축:** 창이 64K 이하인 기본 Ollama에서는 창의 72%에 도달하면 기록 압축을 시작하며, 도구 호출과 결과 쌍은 온전히 유지합니다. 실험 기능인 noa 압축을 켜도 이런 프로필에서는 이 보호 장치가 우선합니다. noa는 건너뛰고 경고를 남깁니다. 32K 이하이면 도구 출력 중 모델에 전달하는 부분을 하나당 16,384**바이트**(16KiB, 유니코드 문자 수가 아니라 SDK의 `len(s)` 기준)로 제한합니다. 32K 초과부터 64K 이하까지는 32,768바이트(32KiB)로 제한합니다. 전체 출력은 도구 결과에서 가리키는 별도 파일에 보존합니다. 클라우드 형식은 기존 압축 및 noa 동작을 유지합니다.
- **승인 판정:** 직접 지정한 판정 프로필이 우선합니다. 지정하지 않았다면 원래 에이전트·실행에서 실제 사용한 네이티브 프로필(바인딩·풀·장애 조치 포함)을 우선하고, 클라우드 실행이나 원래 프로필 정보가 없으면 활성 프로필을 사용합니다. 네이티브 판정 기본값은 **90초**, 실패 처리 방식은 **ask(사용자 확인)**입니다. 직접 저장한 값이 우선하며 사용자 승인 대기는 기본 300초 뒤 거부입니다. 네이티브 출력은 decision/comment JSON 스키마로 제한한 뒤 검증합니다. 설정 화면은 서버의 저장 값을 불러오기 전에는 저장할 수 없습니다.

### 일반 OpenAI 호환 서버(vLLM, LM Studio)

이런 서버에도 기존 형식을 그대로 선택할 수 있습니다. **OpenAI (Chat Completions)**를 고르고 기본 URL을 서버의 `/v1` 엔드포인트로 지정하세요. 네이티브 Ollama 형식은 쓰지 마세요.

- 네이티브 형식의 **모델 불러오기**는 `/api/tags`를 읽고, HTTP 404 또는 405가 돌아오면 `/v1/models`로 대신 조회합니다. 그래서 일반 서버의 모델 목록이 보일 수 있습니다.
- 채팅은 **절대** 조용히 전환하지 않습니다. 네이티브 프로필은 항상 `/api/chat`을 호출하는데, 일반 서버에는 이 경로가 없습니다. 이때 나오는 "모델 또는 엔드포인트를 찾을 수 없음" 오류와 `ollama pull` 안내는 해당되지 않으며, 모델을 내려받아도 vLLM이나 LM Studio 서버 문제는 해결되지 않습니다. 형식을 OpenAI로 바꾸세요.

### 모델 불러오기와 연결 테스트

**모델 불러오기**는 `/api/tags`와 `/api/show`를 읽습니다. 항목마다 매개변수 크기, 양자화 수준, `tools` / `thinking` / `vision` 배지를 표시합니다. 메타데이터에 `tools`가 없는 모델에는 *"경고: 이 모델은 기본 도구 호출 지원을 선언하지 않았습니다. ScopeWeaver 에이전트는 자산 그래프와 탐색 그래프를 갱신하려면 도구 호출이 필요합니다."*가 표시됩니다. 메타데이터를 가져오지 못한 경우에는 미지원이 아니라 알 수 없음으로 표시합니다.

**연결 테스트**는 `/api/show`로 모델을 확인하고, 프로필의 `num_ctx`, `keep_alive`, `num_predict`로 한 차례 도구 호출을 점검한 뒤 `/api/ps`를 읽습니다. 결과 패널에는 할당된 문맥이 요청을 충족하는지, 도구 호출이 돌아왔는지, 초기 로드 시간(밀리초), 생성 속도(초당 토큰)가 표시됩니다. 모델이 없으면 서버에서 실행할 정확한 명령을 보여 줍니다. 예: `ollama pull qwen2.5-coder:32b`. ScopeWeaver는 모델을 내려받거나 Modelfile을 만들지 않습니다.

### 호스트와 컨테이너 경로

| ScopeWeaver 실행 위치 | Ollama 실행 위치 | 기본 URL |
| --- | --- | --- |
| 호스트 | 같은 호스트 | `http://127.0.0.1:11434` |
| Docker Compose | 호스트 | `http://host.docker.internal:11434` |
| Docker Compose | 번들 `ollama` 서비스 | `http://ollama:11434` |

컨테이너 안의 `127.0.0.1`은 컨테이너 자신입니다. Compose는 `host.docker.internal`을 호스트 게이트웨이(`host-gateway`)로 연결합니다. Ollama는 기본적으로 `127.0.0.1:11434`에서 수신하므로, Linux에서는 컨테이너가 접근할 수 있는 주소에서 수신하도록 설정해야 합니다(예: `OLLAMA_HOST=0.0.0.0:11434`). 이 경우 네트워크에 노출되므로 방화벽으로 11434 포트를 제한하세요.

**선택형 Compose 서비스:** `docker compose --profile local-llm up -d`는 `OLLAMA_NO_CLOUD=1`, `OLLAMA_KEEP_ALIVE=30m`, `OLLAMA_NUM_PARALLEL=1`, `OLLAMA_CONTEXT_LENGTH=32768`, `ollama_data` 볼륨으로 `ollama` 서비스를 시작합니다. 모델은 `docker compose exec ollama ollama pull qwen2.5-coder:14b`로 내려받습니다. 이 프로필을 지정하지 않으면 Compose는 이전과 같이 동작합니다. 번들 서비스는 GPU를 요청하지 않습니다. **macOS의 Docker 컨테이너는 Apple GPU를 사용할 수 없어 이 서비스는 CPU로만 실행됩니다.** 실용적인 속도가 필요하면 Mac에 Ollama를 직접 설치하고 호스트 템플릿을 사용하세요.

### 네트워크 정책(`local_llm_only`)

기본값은 꺼짐이며 클라우드 제공자를 계속 사용할 수 있습니다. **시스템 설정 → 로컬 전용 LLM 네트워크** 또는 `SCOPEWEAVER_LOCAL_LLM_ONLY=true`로 켭니다. 켜면 다음과 같이 동작합니다.

- LLM HTTP(추론, 모델 목록, 연결 테스트, 예열, 승인 판정)는 **루프백과 명시적 신뢰 CIDR**(`local_llm_allowed_cidrs`)만 허용합니다. 사설 IPv4, IPv6 ULA, LAN, Docker 주소는 더 이상 자동으로 신뢰하지 않습니다. 직접 관리하는 추론 서버 주소만 추가하고 IPv4는 `/32`, IPv6는 `/128`을 권장합니다.
- 링크 로컬·멀티캐스트·지정되지 않은 주소는 허용 목록에 있어도 거부합니다. 연결을 열 때 DNS가 반환한 모든 주소를 검사하며 전부 허용되어야 합니다. 모든 리디렉션과 LLM 프로필 프록시를 거부합니다.
- **Agent 웹 검색(모든 백엔드), 검색 연결 테스트, 업데이트 확인·다운로드를 차단합니다.** 업데이트 화면은 이 정책을 일반 연결 오류와 구분해 설명합니다. 현재 버전 확인·업데이트 진행 상태·로컬 롤백은 유지하며, 원격 릴리스 캐시로 정책을 우회하지 않습니다.
- 변경은 재시작 없이 이미 생성된 세션에도 적용합니다. 다운로드 시작 후 strict 모드를 켜면 다음 네트워크 동작·응답 본문 읽기 검사에서 중단하고 부분 staging 파일을 정리합니다. 이미 전송한 트래픽은 회수할 수 없습니다.
- `SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS`(또는 기존 `ARTEX_LOCAL_LLM_ALLOWED_CIDRS`)도 함께 적용하며 저장 목록에 복사하거나 UI에서 편집하지 않습니다. 정책 환경 변수의 `true`는 UI에서 끌 수 없습니다.
- 대상 트래픽·대상 도구·전역 프록시는 연결을 유지합니다. 애플리케이션 정책이며 암호학적 보장이나 호스트 전체의 오프라인 보장이 아닙니다.

**기존 strict LAN/Docker 프로필 전환:** 같은 컴퓨터의 `http://127.0.0.1:11434`에는 CIDR이 필요 없습니다. LAN의 `http://192.168.1.20:11434`에는 `192.168.1.20/32`, IPv6 추론 주소 `fd00::20`에는 `fd00::20/128`을 추가하세요. Docker에서는 호스트와 컨테이너가 다른 주소를 볼 수 있으므로 **ScopeWeaver 컨테이너 안에서** 이름을 확인하세요.

```bash
# 기존 Compose 서비스의 실제 이름 해석과 네트워크 주소를 읽습니다.
docker compose exec scopeweaver getent ahosts host.docker.internal
docker compose exec scopeweaver getent ahosts ollama
docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}} {{.GlobalIPv6Address}}{{end}}' "$(docker compose ps -q ollama)"
```

이미지에 `getent`가 없으면 `docker compose exec scopeweaver cat /etc/hosts`로 기존 호스트 게이트웨이 매핑을 확인하고 `docker inspect`로 Ollama 컨테이너 주소를 확인하세요. 예시 주소를 복사하거나 Docker 서브넷 전체를 허용하지 말고 ScopeWeaver가 실제 연결하는 추론 IP만 사용하세요. **추가 신뢰 CIDR** 또는 환경 변수에 추가합니다(예: `SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS=192.168.1.20/32`). `host.docker.internal`과 `ollama`라는 이름 자체는 신뢰 근거가 아닙니다. Docker가 서비스를 다시 만들거나 주소를 바꾸면 좁은 허용 목록을 갱신한 뒤 **모델 불러오기** 또는 **연결 테스트**를 다시 실행하세요.

ScopeWeaver는 Ollama 서버가 이후에 하는 일을 볼 수 없습니다. **Ollama는 요청을 Ollama 클라우드 모델로 전달할 수 있습니다.** 번들 Compose 서비스는 `OLLAMA_NO_CLOUD=1`을 설정하지만, 호스트나 원격 Ollama는 별도로 클라우드를 꺼야 합니다(`OLLAMA_NO_CLOUD=1` 또는 `~/.ollama/server.json`의 `disable_ollama_cloud`, [FAQ](https://docs.ollama.com/faq) 참고).

### 환경 변수와 설정 파일

`SCOPEWEAVER_LLM_*`가 `ARTEX_LLM_*`보다 우선합니다. 명시적으로 비워 둔 새 변수도 이전 변수를 덮어씁니다. 두 변수 모두 `config.json`의 `llm` 블록보다 우선합니다(경로는 `SCOPEWEAVER_CONFIG` 또는 `ARTEX_CONFIG`).

| 접미사 | 의미 |
| --- | --- |
| `PROVIDER` | `anthropic`, `openai`, `openai-responses`, `ollama` |
| `MODEL`, `BASE_URL`, `PROXY` | 모델, 엔드포인트, 선택적 LLM 프록시 |
| `API_KEY` | Anthropic·OpenAI 형식은 기본 URL이 명시적 로컬이 아니면 필수, Ollama는 선택적 토큰 |
| `CONTEXT_WINDOW_K`, `MAX_TOKENS`, `KEEP_ALIVE`, `MAX_CONCURRENT` | 위에서 설명한 설정 |
| `THINKING_TYPE`, `REASONING_EFFORT`, `STREAM` | 기존 추론 및 스트리밍 옵션 |

정책 변수: `SCOPEWEAVER_LOCAL_LLM_ONLY` / `ARTEX_LOCAL_LLM_ONLY`, 쉼표로 구분한 `SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS` / `ARTEX_LOCAL_LLM_ALLOWED_CIDRS`.

시작 시 Anthropic·OpenAI 형식에 공통 키가 설정되어 있지 않으면 `ANTHROPIC_API_KEY` 또는 `OPENAI_API_KEY`를 사용합니다. 인증이 필요한 로컬 게이트웨이도 같습니다. 제공자 키가 설정된 환경에서 키 없는 로컬 프로필로 시작하려면 `SCOPEWEAVER_LLM_API_KEY=`(또는 `ARTEX_LLM_API_KEY=`)를 명시적으로 비워 설정하세요. 설정 파일의 키를 덮어쓰고 제공자 키도 사용하지 않습니다. 네이티브 Ollama는 `OLLAMA_API_KEY`만 사용하며 Anthropic·OpenAI 키를 가져오지 않습니다.

UI에서 새 프로필의 키를 비워 저장하면 제공자 환경 변수의 키를 가져오지 않습니다. **연결 테스트**에서는 프로필 ID가 있으면 해당 프로필의 저장된 키를 사용할 수 있습니다. 프로필 ID 없이 로컬 Anthropic·OpenAI 연결을 테스트할 때는 형식, 정규화된 기본 URL, 프록시가 활성 서비스와 모두 같을 때만 활성 키를 사용합니다. 새 초안을 테스트할 때도 같은 기준입니다. 로컬 엔드포인트, 형식, 프록시 중 하나라도 다르면 활성 키를 가져오지 않으며, 네이티브 Ollama는 활성 클라우드 키를 가져오지 않습니다. 로컬이 아닌 연결 테스트는 기존처럼 활성 키를 사용할 수 있습니다.

```bash
# Docker의 ScopeWeaver가 호스트 Ollama를 사용하는 .env 예
# your-installed-tool-model은 모델 불러오기에 나온 도구 지원 태그로 바꾸세요.
SCOPEWEAVER_LLM_PROVIDER=ollama
SCOPEWEAVER_LLM_BASE_URL=http://host.docker.internal:11434
SCOPEWEAVER_LLM_MODEL=your-installed-tool-model
SCOPEWEAVER_LLM_CONTEXT_WINDOW_K=32
SCOPEWEAVER_LLM_MAX_CONCURRENT=1
SCOPEWEAVER_LLM_KEEP_ALIVE=30m
SCOPEWEAVER_LLM_MAX_TOKENS=4096
SCOPEWEAVER_LOCAL_LLM_ONLY=false   # strict 모드 전에 실제 호스트 /32 또는 /128 추가
# SCOPEWEAVER_LOCAL_LLM_ALLOWED_CIDRS=192.168.1.20/32  # 실제 추론 IP로 교체
```

```json
{
  "llm": { "provider": "ollama", "base_url": "http://127.0.0.1:11434", "model": "your-installed-tool-model", "context_window_k": 32, "llm_max_concurrent": 1, "keep_alive": "30m", "max_tokens": 4096 },
  "local_llm_only": false,
  "local_llm_allowed_cidrs": []
}
```

### 진단과 계측의 범위

**연결 테스트**는 사용자가 누르면 추론 검사를 실행하며 템플릿 선택은 설치된 메타데이터만 조회합니다. 도구·문맥 확인, 로드 시간·속도와 함께 선택한 실행 모델의 `/api/ps` `size`와 `size_vram`으로 GPU 메모리 비율을 표시합니다. 전체 크기가 없거나 0이거나 VRAM 크기가 없으면 0%가 아닌 **알 수 없음**입니다. 양수 전체 크기에 VRAM이 명시적으로 0이면 0%입니다. 이 값은 메모리 추정치이며 정확한 가중치 분할·GPU 용량 측정·모델 상주 보장이 아닙니다.

80% 이하면 CPU 처리로 생성이 느려질 수 있다고 경고합니다. 다른 모델이 로드되어 있거나 같은 엔드포인트의 명시적 판정 프로필이 다른 모델·문맥을 쓰면 메모리·재로드 경고를 각각 표시합니다. 같은 모델·문맥을 써도 상주를 보장할 수 없습니다. 진단은 모델을 내려받거나 프로필을 저장·활성화하지 않습니다.

문맥 한계 근접 경고는 보고된 토큰 수에 근거한 서버 로그의 경험적 신호이며 잘림 감지기가 아닙니다. Ollama 원본 타이밍 필드는 원본 기록이 켜진 경우에만 저장하며 토큰·캐시 사용량은 별도로 항상 계측합니다. 작은 모델에는 여전히 작업별 검증이 필요하며 아래의 기존 신뢰성 한계는 유지됩니다.

### 하드웨어 등급

GPU VRAM 또는 Apple 통합 메모리 기준의 대략적인 출발점이며, 아래의 도구 호출 지원 카탈로그 모델을 사용합니다. **성능 측정값이 아니라 추정치입니다.** 문맥 값은 프로필에 설정하는 **문맥 창(K)**이며, 모델 자체의 문맥 한도와 메모리(가중치 + 문맥 길이와 `OLLAMA_NUM_PARALLEL`에 따라 늘어나는 KV 캐시)에 모두 맞아야 합니다. 요청한 문맥이 실제로 할당됐는지는 **연결 테스트**로 확인하세요.

| 메모리 | 예시 모델 | 문맥 창(K) | 역할 |
| --- | --- | --- | --- |
| 8–12GB | `qwen2.5-coder:7b`, `qwen3:8b` | `16`–`32` | 승인 판정 또는 보조 프로필. 주 에이전트로는 약함 |
| 16–24GB | `qwen2.5-coder:14b`, `qwen3:14b` | `32` | 워크스테이션의 주 에이전트 |
| 32–48GB | `qwen2.5-coder:32b`, `qwen3:32b` | `32` | 주 에이전트. `64`는 모델 한도가 허용할 때만 사용. 이 모델들은 32K와 40K |
| 64–128GB 이상 | `llama3.3:70b` | `64`(모델 한도 128K) | 메모리가 큰 호스트의 주 에이전트. 64K는 이 범위의 상단이 필요 |

템플릿은 선택한 서버에 설치된 모델 중에서 고르며, 문맥은 32K를 사용합니다. 하드웨어 예시는 모델 자동 선택 기준이 아닙니다.

### 모델 후보

[Ollama 라이브러리](https://ollama.com/library) 페이지에 `tools` 기능이 표시된 모델만 실었습니다(2026-10-15 확인). 내려받기 크기는 기본 양자화 기준입니다. **메모리 안내는 추정치입니다.** 가중치에 더해 문맥 길이와 동시 요청 수에 따라 늘어나는 KV 캐시가 필요합니다. ScopeWeaver는 이 모델과 하드웨어를 성능 측정하지 않았으며, 도구 호출 품질은 모델마다 다릅니다.

| 모델 | 내려받기 | 참고 |
| --- | --- | --- |
| `qwen2.5-coder:7b` | 4.7GB | 실용적인 가장 작은 후보, 승인 판정 후보 |
| `qwen2.5-coder:14b` | 9.0GB | 워크스테이션 후보 |
| `qwen2.5-coder:32b` | 20GB | 메모리가 큰 호스트 후보 |
| `qwen3:8b`, `qwen3:14b`, `qwen3:32b` | 5.2 / 9.3 / 20GB | `thinking`도 선언 |
| `llama3.3:70b` | 43GB | 메모리가 큰 호스트 |

`qwen3:72b`와 `llama3.2:8b`는 Ollama 라이브러리에 없습니다. `llama3.2`는 1B와 3B로 제공됩니다. 사용하기 전에 자신의 서버에서 **모델 불러오기**로 `tools`를 확인하세요.

**관찰된 한계(2026-10-15):** `tools` 기능을 선언했다고 해서 실제 도구 호출이 보장되지는 않습니다. 브랜치 테스트에서 `qwen2.5-coder:7b`는 도구 호출처럼 보이는 JSON을 일반 텍스트로 써서 도구가 실행되지 않았고, `qwen3:4b`는 도구 호출 진단은 통과했지만 긴 자율 작업을 완료하지 못했습니다. 작은 모델은 주 에이전트로 검증되지 않은 것으로 보세요. [검증 결과](VERIFICATION.md#자율-작업-신뢰성은-검증되지-않음)를 참고하세요.

[README](../../README.ko.md)의 사용 제한은 그대로 적용됩니다. 어떤 제공자를 선택하든 ScopeWeaver는 로컬에 격리된 환경에서만 실행하세요.

[English guide](../llm-providers.md)
