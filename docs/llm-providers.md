# LLM provider templates

ScopeWeaver uses its existing provider formats and custom base URL fields. A template only fills a **new profile's form**; it does not contact a provider, save credentials, activate a profile, or modify existing profiles.

## Z.ai GLM-5.3

![GLM-5.3 new-profile template](../screenshots/en/llm-templates.png)

Template preview with no API key entered; this is not a live provider connection.

Open **LLM → New → Configuration template**. Choose **General API (recommended)** for API billing, or **Coding Plan (reference)** only after obtaining Z.ai authorization for ScopeWeaver.

| Field | General API | Coding Plan reference |
| --- | --- | --- |
| Format | OpenAI Chat Completions | OpenAI Chat Completions |
| Base URL | `https://api.z.ai/api/paas/v4` | `https://api.z.ai/api/coding/paas/v4` |
| Model | `glm-5.3` | `glm-5.3` |
| Thinking | `enabled` | `enabled` |
| Reasoning effort | `max` | `max` |
| Context window (K) | `1000` | `1000` |
| Output limit | `0` (omit; provider default) | `0` (omit; provider default) |

The endpoints and billing are distinct: General API usage is separate from Coding Plan subscription quota. See [Z.ai connection configuration](https://zcode.z.ai/en/docs/configuration). Confirm your account's model access and billing before testing.

GLM-5.3 accepts text, has a 1M-token context window, and requires reasoning to remain enabled. Supported effort values are `low`, `high`, and `max`; this template uses `max`. The model field remains editable, so review reasoning and context settings when choosing another model. See the [official GLM-5.3 guide](https://docs.z.ai/guides/llm/glm-5.3).

**Coding Plan support boundary:** Z.ai limits Coding Plan to officially supported tools. ScopeWeaver is not on that list; an endpoint template does not establish permission or support. Obtain Z.ai authorization before using it here. See [tool integration](https://docs.z.ai/devpack/tool/others) and [usage policy](https://docs.z.ai/devpack/usage-policy). ScopeWeaver does not impersonate supported tools or switch billing endpoints automatically.

Enter a profile name and your own API key. Applying either template preserves any name, key, proxy, session-header setting, and other unrelated preferences already entered. Save when ready; a new profile must be activated separately. The existing **Test connection** and **Load models** controls make real provider requests only when clicked; this change does not perform a live provider test.

Verified against official documentation on **2026-10-03**. Provider availability, account entitlement, and policies may change.

## OpenRouter

Format `openrouter` sends OpenAI Chat Completions requests to OpenRouter (`https://openrouter.ai/api/v1` by default) with Bearer auth and OpenRouter's own API key (`OPENROUTER_API_KEY` when configuring from the environment). Model ids are OpenRouter's `vendor/model` slugs, e.g. `anthropic/claude-sonnet-4.5`; **Load models** reads OpenRouter's model list. Output-limit and retry settings behave like the `openai` format.

Two profile fields control OpenRouter provider routing — the request body's `provider` object. They apply only to the `openrouter` format; saving any other format clears them.

| Field | Wire field | Meaning |
| --- | --- | --- |
| Provider order | `provider.order` | Comma-separated provider slugs tried in order (e.g. `anthropic, google`). Blank keeps OpenRouter's price-based load balancing. |
| Fallbacks | `provider.allow_fallbacks` | Allowed (default) keeps backup providers; Disallowed pins each request to the listed providers only. "Do not send" omits the field and keeps OpenRouter's default. |

The connection test sends the same routing object a real conversation would, so a rejected model/provider combination fails the test instead of a live run. Failover between ScopeWeaver profiles is a separate mechanism (rotation) and unaffected. See [OpenRouter provider routing](https://openrouter.ai/docs/features/provider-routing).

[한국어 안내](ko/llm-providers.md)
