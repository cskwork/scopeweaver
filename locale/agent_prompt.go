package locale

func init() {
	RegisterAll(map[string]map[Lang]string{
		"[prompt] agent=%q: template rendering failed; using role default: %v": {
			En:     "[prompt] agent=%q: template rendering failed; using role default: %v",
			Ko:     "[prompt] agent=%q: 템플릿 렌더링 실패; 역할 기본 프롬프트 사용: %v",
			ZhHans: "[prompt] agent=%q：模板渲染失败；使用该角色的默认提示词：%v",
		},
	})
}
