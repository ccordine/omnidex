package catalog

var providerDefinitions = []Definition{
	{
		ID: "ollama", DisplayName: "Ollama", Aliases: []string{"local"}, Protocol: ProtocolOllama,
		EnvironmentPrefixes:           []string{"OLLAMA", "OMNI"},
		BaseURLEnvironmentKeys:        []string{"OLLAMA_BASE_URL"},
		EmbeddingModelEnvironmentKeys: []string{"OLLAMA_EMBEDDING_MODEL"},
		DefaultEmbeddingModel:         "nomic-embed-text",
		SupportsExactPreparedStations: true, SupportsEmbeddings: true,
	},
	{
		ID: "openai", DisplayName: "OpenAI", Aliases: []string{"chatgpt", "chat-gpt"}, Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefixes: []string{"OPENAI"}, APIKeyEnvironmentKeys: []string{"OPENAI_API_KEY"}, BaseURLEnvironmentKeys: []string{"OPENAI_BASE_URL"},
		DefaultBaseURL: "https://api.openai.com/v1", DefaultEmbeddingModel: "text-embedding-3-small",
		SupportsEmbeddings: true,
	},
	{
		ID: "azure", DisplayName: "Microsoft Azure AI", Aliases: []string{"azureai", "azure-ai", "azure-openai", "azure_openai", "microsoft", "msai", "windows", "windowsai", "windows-ai"}, Protocol: ProtocolAzure,
		EnvironmentPrefixes: []string{"AZURE_AI", "AZURE_OPENAI"}, APIKeyEnvironmentKeys: []string{"AZURE_AI_API_KEY", "AZURE_OPENAI_API_KEY"}, BaseURLEnvironmentKeys: []string{"AZURE_AI_BASE_URL", "AZURE_OPENAI_ENDPOINT", "AZURE_OPENAI_BASE_URL"},
		EmbeddingModelEnvironmentKeys: []string{"AZURE_AI_EMBEDDING_MODEL", "AZURE_OPENAI_EMBEDDING_DEPLOYMENT"},
		SupportsEmbeddings:            true, RequiresBaseURL: true,
	},
	{
		ID: "xai", DisplayName: "xAI", Aliases: []string{"x-ai", "grok", "grock"}, Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefixes: []string{"XAI", "GROK", "GROCK"}, APIKeyEnvironmentKeys: []string{"XAI_API_KEY", "GROK_API_KEY"}, BaseURLEnvironmentKeys: []string{"XAI_BASE_URL", "GROK_BASE_URL"},
		DefaultBaseURL: "https://api.x.ai/v1",
	},
	{
		ID: "google", DisplayName: "Google Gemini", Aliases: []string{"gemini", "googleai", "google-ai"}, Protocol: ProtocolGoogle,
		EnvironmentPrefixes: []string{"GOOGLE", "GEMINI"}, APIKeyEnvironmentKeys: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}, BaseURLEnvironmentKeys: []string{"GOOGLE_BASE_URL"},
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", DefaultEmbeddingModel: "text-embedding-004",
		SupportsEmbeddings: true,
	},
	{
		ID: "anthropic", DisplayName: "Anthropic", Aliases: []string{"claude"}, Protocol: ProtocolAnthropic,
		EnvironmentPrefixes: []string{"ANTHROPIC", "CLAUDE"}, APIKeyEnvironmentKeys: []string{"ANTHROPIC_API_KEY"}, BaseURLEnvironmentKeys: []string{"ANTHROPIC_BASE_URL"},
		DefaultBaseURL: "https://api.anthropic.com/v1",
	},
	{
		ID: "huggingface", DisplayName: "Hugging Face", Aliases: []string{"hugging-face", "hf"}, Protocol: ProtocolHuggingFace,
		EnvironmentPrefixes: []string{"HUGGINGFACE", "HF"}, APIKeyEnvironmentKeys: []string{"HUGGINGFACE_API_KEY", "HF_TOKEN"}, BaseURLEnvironmentKeys: []string{"HUGGINGFACE_BASE_URL"},
		DefaultBaseURL: "https://router.huggingface.co", DefaultEmbeddingModel: "sentence-transformers/all-mpnet-base-v2",
		SupportsEmbeddings: true,
	},
	compatibleDefinition("deepseek", "DeepSeek", []string{"deep-seek"}, []string{"DEEPSEEK"}, "https://api.deepseek.com", false),
	compatibleDefinition("qwen", "Alibaba Qwen / Model Studio", []string{"dashscope", "tongyi", "alibaba", "alibaba-qwen"}, []string{"QWEN", "DASHSCOPE"}, "https://dashscope.aliyuncs.com/compatible-mode/v1", true),
	compatibleDefinition("moonshot", "Moonshot AI / Kimi", []string{"kimi", "moonshot-ai"}, []string{"MOONSHOT", "KIMI"}, "https://api.moonshot.ai/v1", false),
	compatibleDefinition("zhipu", "Zhipu AI / BigModel", []string{"glm", "bigmodel", "zhipuai"}, []string{"ZHIPU", "BIGMODEL", "GLM"}, "https://open.bigmodel.cn/api/paas/v4", true),
	compatibleDefinition("zai", "Z.AI", []string{"z-ai"}, []string{"ZAI"}, "https://api.z.ai/api/paas/v4", false),
	compatibleDefinition("minimax", "MiniMax", []string{"mini-max"}, []string{"MINIMAX"}, "https://api.minimax.io/v1", false),
	compatibleDefinition("qianfan", "Baidu Qianfan / ERNIE", []string{"baidu", "ernie", "baidu-qianfan"}, []string{"QIANFAN", "BAIDU", "ERNIE"}, "https://qianfan.baidubce.com/v2", true),
	compatibleDefinition("hunyuan", "Tencent Hunyuan", []string{"tencent", "tencent-hunyuan"}, []string{"HUNYUAN", "TENCENT_HUNYUAN"}, "https://api.hunyuan.cloud.tencent.com/v1", true),
	compatibleDefinition("doubao", "ByteDance Doubao / Volcengine Ark", []string{"volcengine", "ark", "bytedance", "volcengine-ark"}, []string{"DOUBAO", "VOLCENGINE", "ARK"}, "https://ark.cn-beijing.volces.com/api/v3", false),
	compatibleDefinition("stepfun", "StepFun", []string{"step", "step-fun"}, []string{"STEPFUN", "STEP"}, "https://api.stepfun.com/v1", false),
	compatibleDefinition("yi", "01.AI Yi", []string{"01ai", "01-ai", "lingyiwanwu"}, []string{"YI", "LINGYIWANWU"}, "https://api.01.ai/v1", false),
	compatibleDefinition("baichuan", "Baichuan AI", []string{"baichuan-ai"}, []string{"BAICHUAN"}, "https://api.baichuan-ai.com/v1", true),
	compatibleDefinition("spark", "iFlytek Spark", []string{"iflytek", "xfyun"}, []string{"SPARK", "IFLYTEK", "XFYUN"}, "https://spark-api-open.xf-yun.com/v1", false),
	compatibleDefinition("siliconflow", "SiliconFlow", []string{"silicon-flow"}, []string{"SILICONFLOW"}, "https://api.siliconflow.cn/v1", true),
	compatibleDefinition("modelscope", "ModelScope API Inference", []string{"model-scope"}, []string{"MODELSCOPE"}, "https://api-inference.modelscope.cn/v1", false),
	compatibleDefinition("modelarts", "Huawei ModelArts MaaS", []string{"huawei", "huawei-maas", "modelarts-maas"}, []string{"MODELARTS", "HUAWEI_MAAS"}, "https://api.modelarts-maas.com/openai/v1", false),
	compatibleDefinition("mimo", "Xiaomi MiMo", []string{"xiaomi", "xiaomi-mimo"}, []string{"MIMO", "XIAOMI_MIMO"}, "https://api.xiaomimimo.com/v1", false),
	compatibleDefinition("longcat", "Meituan LongCat", []string{"long-cat", "meituan", "meituan-longcat"}, []string{"LONGCAT", "MEITUAN_LONGCAT"}, "https://api.longcat.chat/openai/v1", false),
	compatibleDefinition("antling", "Ant Ling / InclusionAI", []string{"ant-ling", "ling", "inclusionai", "inclusion-ai"}, []string{"ANTLING", "ANT_LING", "INCLUSIONAI"}, "https://api.ant-ling.com/v1", false),
	compatibleDefinition("tokenhub", "Tencent TokenHub", []string{"tencent-tokenhub", "tencent-maas"}, []string{"TOKENHUB", "TENCENT_TOKENHUB"}, "https://tokenhub.tencentmaas.com/v1", false),
	{
		ID: "compatible", DisplayName: "Custom OpenAI-compatible", Aliases: []string{"custom", "custom-openai", "openai-compatible"}, Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefixes: []string{"COMPATIBLE"}, APIKeyEnvironmentKeys: []string{"COMPATIBLE_API_KEY"}, BaseURLEnvironmentKeys: []string{"COMPATIBLE_BASE_URL"},
		SupportsEmbeddings: true, RequiresBaseURL: true,
	},
}

func compatibleDefinition(id, displayName string, aliases, prefixes []string, baseURL string, supportsEmbeddings bool) Definition {
	apiKeys := make([]string, 0, len(prefixes))
	baseURLs := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		apiKeys = append(apiKeys, prefix+"_API_KEY")
		baseURLs = append(baseURLs, prefix+"_BASE_URL")
	}
	return Definition{
		ID: id, DisplayName: displayName, Aliases: aliases, Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefixes: prefixes, APIKeyEnvironmentKeys: apiKeys, BaseURLEnvironmentKeys: baseURLs,
		DefaultBaseURL: baseURL, SupportsEmbeddings: supportsEmbeddings, ChineseService: true,
	}
}
