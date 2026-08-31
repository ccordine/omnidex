package catalog

var providerDefinitions = []Definition{
	{
		ID: "ollama", DisplayName: "Ollama", Protocol: ProtocolOllama,
		EnvironmentPrefix:             "OLLAMA",
		DefaultEmbeddingModel:         "nomic-embed-text",
		SupportsExactPreparedStations: true, SupportsEmbeddings: true,
	},
	{
		ID: "openai", DisplayName: "OpenAI", Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefix: "OPENAI",
		DefaultBaseURL: "https://api.openai.com/v1", DefaultEmbeddingModel: "text-embedding-3-small",
		SupportsEmbeddings: true,
	},
	{
		ID: "azure", DisplayName: "Microsoft Azure AI", Protocol: ProtocolAzure,
		EnvironmentPrefix:  "AZURE_AI",
		SupportsEmbeddings: true, RequiresBaseURL: true,
	},
	{
		ID: "xai", DisplayName: "xAI", Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefix: "XAI",
		DefaultBaseURL: "https://api.x.ai/v1",
	},
	{
		ID: "google", DisplayName: "Google Gemini", Protocol: ProtocolGoogle,
		EnvironmentPrefix: "GOOGLE",
		DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", DefaultEmbeddingModel: "text-embedding-004",
		SupportsEmbeddings: true,
	},
	{
		ID: "anthropic", DisplayName: "Anthropic", Protocol: ProtocolAnthropic,
		EnvironmentPrefix: "ANTHROPIC",
		DefaultBaseURL: "https://api.anthropic.com/v1",
	},
	{
		ID: "huggingface", DisplayName: "Hugging Face", Protocol: ProtocolHuggingFace,
		EnvironmentPrefix: "HUGGINGFACE",
		DefaultBaseURL: "https://router.huggingface.co", DefaultEmbeddingModel: "sentence-transformers/all-mpnet-base-v2",
		SupportsEmbeddings: true,
	},
	compatibleDefinition("deepseek", "DeepSeek", "DEEPSEEK", "https://api.deepseek.com", false),
	compatibleDefinition("qwen", "Alibaba Qwen / Model Studio", "QWEN", "https://dashscope.aliyuncs.com/compatible-mode/v1", true),
	compatibleDefinition("moonshot", "Moonshot AI / Kimi", "MOONSHOT", "https://api.moonshot.ai/v1", false),
	compatibleDefinition("zhipu", "Zhipu AI / BigModel", "ZHIPU", "https://open.bigmodel.cn/api/paas/v4", true),
	compatibleDefinition("zai", "Z.AI", "ZAI", "https://api.z.ai/api/paas/v4", false),
	compatibleDefinition("minimax", "MiniMax", "MINIMAX", "https://api.minimax.io/v1", false),
	compatibleDefinition("qianfan", "Baidu Qianfan / ERNIE", "QIANFAN", "https://qianfan.baidubce.com/v2", true),
	compatibleDefinition("hunyuan", "Tencent Hunyuan", "HUNYUAN", "https://api.hunyuan.cloud.tencent.com/v1", true),
	compatibleDefinition("doubao", "ByteDance Doubao / Volcengine Ark", "DOUBAO", "https://ark.cn-beijing.volces.com/api/v3", false),
	compatibleDefinition("stepfun", "StepFun", "STEPFUN", "https://api.stepfun.com/v1", false),
	compatibleDefinition("yi", "01.AI Yi", "YI", "https://api.01.ai/v1", false),
	compatibleDefinition("baichuan", "Baichuan AI", "BAICHUAN", "https://api.baichuan-ai.com/v1", true),
	compatibleDefinition("spark", "iFlytek Spark", "SPARK", "https://spark-api-open.xf-yun.com/v1", false),
	compatibleDefinition("siliconflow", "SiliconFlow", "SILICONFLOW", "https://api.siliconflow.cn/v1", true),
	compatibleDefinition("modelscope", "ModelScope API Inference", "MODELSCOPE", "https://api-inference.modelscope.cn/v1", false),
	compatibleDefinition("modelarts", "Huawei ModelArts MaaS", "MODELARTS", "https://api.modelarts-maas.com/openai/v1", false),
	compatibleDefinition("mimo", "Xiaomi MiMo", "MIMO", "https://api.xiaomimimo.com/v1", false),
	compatibleDefinition("longcat", "Meituan LongCat", "LONGCAT", "https://api.longcat.chat/openai/v1", false),
	compatibleDefinition("antling", "Ant Ling / InclusionAI", "ANTLING", "https://api.ant-ling.com/v1", false),
	compatibleDefinition("tokenhub", "Tencent TokenHub", "TOKENHUB", "https://tokenhub.tencentmaas.com/v1", false),
	{
		ID: "compatible", DisplayName: "Custom OpenAI-compatible", Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefix: "COMPATIBLE",
		SupportsEmbeddings: true, RequiresBaseURL: true,
	},
}

func compatibleDefinition(id, displayName, environmentPrefix, baseURL string, supportsEmbeddings bool) Definition {
	return Definition{
		ID: id, DisplayName: displayName, Protocol: ProtocolOpenAICompatible,
		EnvironmentPrefix: environmentPrefix,
		DefaultBaseURL: baseURL, SupportsEmbeddings: supportsEmbeddings, ChineseService: true,
	}
}
