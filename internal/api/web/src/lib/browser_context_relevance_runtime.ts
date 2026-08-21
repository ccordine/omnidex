import type { MLCEngineInterface } from "@mlc-ai/web-llm";
import {
  browserContextCompletionRequest,
  browserContextFailure,
  browserContextProviderResult,
  browserContextSuccess,
  requireBrowserContextConfig,
  requireBrowserContextJob,
  type BrowserContextConfig,
} from "./browser_context_relevance_protocol";

type BrowserInferenceState = "disabled" | "loading" | "ready" | "running" | "error";

export class BrowserContextRelevanceRuntime {
  private readonly abort = new AbortController();
  private engine: MLCEngineInterface | null = null;
  private worker: Worker | null = null;
  private socket: WebSocket | null = null;
  private started = false;
  private messageChain = Promise.resolve();

  async start(): Promise<void> {
    if (this.started) throw new Error("Browser context relevance runtime was started twice.");
    this.started = true;
    const config = await this.loadConfig();
    if (!config.enabled) {
      this.publish("disabled", "Browser context relevance is server-executed.");
      return;
    }
    if (!("gpu" in navigator)) {
      throw new Error("Configured browser context relevance requires WebGPU.");
    }
    const model = config.model as string;
    this.publish("loading", `Loading ${model}`);
    const webllm = await import("@mlc-ai/web-llm");
    if (!webllm.prebuiltAppConfig.model_list.some((candidate) => candidate.model_id === model)) {
      throw new Error(`Configured browser model ${model} is not registered by this WebLLM build.`);
    }
    this.worker = new Worker(new URL("../workers/browser_inference_worker.ts", import.meta.url), {
      type: "module",
    });
    this.engine = await webllm.CreateWebWorkerMLCEngine(
      this.worker,
      model,
      {
        appConfig: { ...webllm.prebuiltAppConfig, cacheBackend: "opfs" },
        initProgressCallback: (report) => this.publish("loading", report.text),
        logLevel: "WARN",
      },
    );
    if (this.abort.signal.aborted) {
      await this.releaseEngine();
      return;
    }
    this.publish("ready", `${model} is resident in browser WebGPU.`);
    await this.runSocket(config);
  }

  stop(): void {
    this.abort.abort();
    this.socket?.close(1000, "page disconnected");
    this.socket = null;
    void this.releaseEngine();
  }

  private async loadConfig(): Promise<BrowserContextConfig> {
    const response = await fetch("/v1/browser-inference/context-relevance", {
      method: "GET",
      signal: this.abort.signal,
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error(`Browser context relevance config failed with HTTP ${response.status}.`);
    }
    return requireBrowserContextConfig(await response.json());
  }

  private async runSocket(config: BrowserContextConfig): Promise<void> {
    const socket = new WebSocket(browserInferenceWebSocketURL());
    this.socket = socket;
    await waitForSocketOpen(socket, this.abort.signal);
    await new Promise<void>((resolve, reject) => {
      socket.addEventListener("message", (event) => {
        this.messageChain = this.messageChain
          .then(() => this.executeMessage(socket, config, event.data))
          .catch((error) => {
            socket.close(1008, "browser inference failed");
            reject(error);
          });
      });
      socket.addEventListener("error", () => reject(new Error("Browser inference WebSocket failed.")), { once: true });
      socket.addEventListener("close", (event) => {
        if (this.abort.signal.aborted || event.code === 1000) {
          resolve();
          return;
        }
        reject(new Error(`Browser inference WebSocket closed (${event.code} ${event.reason}).`));
      }, { once: true });
    });
  }

  private async executeMessage(
    socket: WebSocket,
    config: BrowserContextConfig,
    rawMessage: unknown,
  ): Promise<void> {
    if (typeof rawMessage !== "string") {
      throw new Error("Browser context relevance job must be one JSON text message.");
    }
    const job = requireBrowserContextJob(JSON.parse(rawMessage), config.model as string);
    let submission;
    try {
      if (!this.engine) throw new Error("Browser WebGPU model is unavailable.");
      this.publish("running", `Executing ${job.station}`);
      await this.engine.resetChat(false);
      const completion = await this.engine.chatCompletion(browserContextCompletionRequest(job));
      const content = completion.choices[0]?.message.content;
      if (typeof content !== "string") {
        throw new Error("WebLLM returned no raw text result.");
      }
      submission = browserContextSuccess(job, browserContextProviderResult(content));
      this.publish("ready", `${job.model} completed ${job.station}.`);
    } catch (error) {
      submission = browserContextFailure(job, error);
      this.publish("error", submission.error as string);
    }
    if (socket.readyState !== WebSocket.OPEN) {
      throw new Error("Browser inference socket closed before result submission.");
    }
    socket.send(JSON.stringify(submission));
  }

  private async releaseEngine(): Promise<void> {
    const engine = this.engine;
    this.engine = null;
    if (engine) {
      try {
        await engine.unload();
      } catch (error) {
        console.error("Browser WebGPU model unload failed", error);
      }
    }
    this.worker?.terminate();
    this.worker = null;
  }

  private publish(state: BrowserInferenceState, detail: string): void {
    document.dispatchEvent(new CustomEvent("omni:browser-inference-status", {
      detail: { station: "context_relevance", state, message: detail },
    }));
  }
}

function browserInferenceWebSocketURL(): string {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${protocol}//${window.location.host}/v1/browser-inference/context-relevance/ws`;
}

function waitForSocketOpen(socket: WebSocket, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const abort = () => {
      socket.close(1000, "startup canceled");
      reject(signal.reason ?? new Error("Browser inference startup was canceled."));
    };
    if (signal.aborted) {
      abort();
      return;
    }
    signal.addEventListener("abort", abort, { once: true });
    socket.addEventListener("open", () => {
      signal.removeEventListener("abort", abort);
      resolve();
    }, { once: true });
    socket.addEventListener("error", () => {
      signal.removeEventListener("abort", abort);
      reject(new Error("Browser inference WebSocket could not connect."));
    }, { once: true });
  });
}
