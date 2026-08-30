package worker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingBrowserHostCapability struct {
	ID        string
	Purpose   string
	API       string
	Source    string
	Driver    string
	CallNames []string
}

var directCodingBrowserHostCapabilityRegistry = []directCodingBrowserHostCapability{
	{
		ID: "runtime.browser.audio_samples",
		Purpose: "Play one bounded sequence of numeric PCM samples through the browser's audible " +
			"audio output during a user interaction.",
		API: "function playBrowserAudioSamples(samples: readonly number[], sampleRateHz: number): void",
		Source: `export function playBrowserAudioSamples(samples: readonly number[], sampleRateHz: number): void {
  if (!Number.isInteger(sampleRateHz) || sampleRateHz < 8000 || sampleRateHz > 96000) {
    throw new Error('Browser audio sample rate must be an integer from 8000 through 96000 Hz.');
  }
  if (samples.length < 1 || samples.length > sampleRateHz * 30) {
    throw new Error('Browser audio output requires between one sample and thirty seconds of samples.');
  }
  const owned = Array.from(samples);
  for (let index = 0; index < owned.length; index += 1) {
    const sample = owned[index];
    if (sample === undefined || !Number.isFinite(sample) || sample < -1 || sample > 1) {
      throw new Error('Browser audio samples must be finite numbers from -1 through 1.');
    }
  }
  publishBrowserHostRequest("runtime.browser.audio_samples", Object.freeze({
    samples: Object.freeze(owned),
    sampleRateHz,
  }));
}`,
		Driver: `registerBrowserHostHandler("runtime.browser.audio_samples", (payload: unknown): void => {
  if (typeof payload !== 'object' || payload === null) {
    throw new Error('Browser audio host received an invalid request payload.');
  }
  const request = payload as { readonly samples?: unknown; readonly sampleRateHz?: unknown };
  if (!Array.isArray(request.samples) || typeof request.sampleRateHz !== 'number') {
    throw new Error('Browser audio host received an invalid request payload.');
  }
  const context = new AudioContext({ sampleRate: request.sampleRateHz });
  const buffer = context.createBuffer(1, request.samples.length, request.sampleRateHz);
  const channel = buffer.getChannelData(0);
  for (let index = 0; index < request.samples.length; index += 1) {
    channel[index] = request.samples[index] as number;
  }
  const output = context.createBufferSource();
  output.buffer = buffer;
  output.connect(context.destination);
  output.addEventListener('ended', () => { void context.close(); }, { once: true });
  const resumed = context.resume();
  output.start();
  void resumed.catch((error: unknown) => {
    console.error('Browser audio output failed.', error);
    void context.close();
  });
});`,
		CallNames: []string{"playBrowserAudioSamples"},
	},
}

func registeredDirectCodingBrowserHostCapabilities() (
	[]directCodingBrowserHostCapability,
	error,
) {
	capabilities := append(
		[]directCodingBrowserHostCapability(nil),
		directCodingBrowserHostCapabilityRegistry...,
	)
	sort.Slice(capabilities, func(left, right int) bool {
		return capabilities[left].ID < capabilities[right].ID
	})
	seenIDs := make(map[string]struct{}, len(capabilities))
	seenCalls := make(map[string]string)
	for index, capability := range capabilities {
		if len(capability.CallNames) != 1 {
			return nil, fmt.Errorf(
				"browser host capability %d must expose exactly one bounded call", index,
			)
		}
		callName := capability.CallNames[0]
		if !directCodingRuntimeCapabilityIDPattern.MatchString(capability.ID) ||
			capability.ID != strings.TrimSpace(capability.ID) {
			return nil, fmt.Errorf("browser host capability %d has invalid ID %q", index, capability.ID)
		}
		if _, duplicate := seenIDs[capability.ID]; duplicate {
			return nil, fmt.Errorf("browser host capability ID %q is registered more than once", capability.ID)
		}
		seenIDs[capability.ID] = struct{}{}
		if previous, duplicate := seenCalls[callName]; duplicate {
			return nil, fmt.Errorf(
				"browser host capabilities %s and %s expose the same call %s",
				previous, capability.ID, callName,
			)
		}
		seenCalls[callName] = capability.ID
		if _, forbidden := directCodingBrowserForbiddenRuntimeHostIdentifiers[callName]; forbidden ||
			directCodingBrowserRuntimeGlobalPermitted(callName) {
			return nil, fmt.Errorf(
				"browser host capability %s call %s collides with ambient runtime authority",
				capability.ID, callName,
			)
		}
		if len(capability.API) > 512 || len(capability.Source) > 8*1024 ||
			len(capability.Driver) > 8*1024 {
			return nil, fmt.Errorf("browser host capability %s exceeds source bounds", capability.ID)
		}
		if strings.TrimSpace(capability.Driver) == "" {
			return nil, fmt.Errorf("browser host capability %s omits its static driver", capability.ID)
		}
		fragment, err := assemblyline.ParseTypeScriptFunction(
			assemblyline.TypeScriptFunctionContract{Signature: capability.API},
			strings.TrimPrefix(strings.TrimSpace(capability.Source), "export "),
		)
		if err != nil {
			return nil, fmt.Errorf("browser host capability %s source: %w", capability.ID, err)
		}
		if fragment.Name != callName {
			return nil, fmt.Errorf(
				"browser host capability %s API names %s but registers %s",
				capability.ID, fragment.Name, callName,
			)
		}
		publishCall := "publishBrowserHostRequest(" + strconv.Quote(capability.ID) + ","
		if strings.Count(capability.Source, "publishBrowserHostRequest(") != 1 ||
			strings.Count(capability.Source, publishCall) != 1 {
			return nil, fmt.Errorf(
				"browser host capability %s wrapper must publish exactly one request under its own ID",
				capability.ID,
			)
		}
		registerCall := "registerBrowserHostHandler(" + strconv.Quote(capability.ID) + ","
		if strings.Count(capability.Driver, "registerBrowserHostHandler(") != 1 ||
			strings.Count(capability.Driver, registerCall) != 1 {
			return nil, fmt.Errorf(
				"browser host capability %s driver must register exactly one handler under its own ID",
				capability.ID,
			)
		}
		if err := assemblyline.ValidateTypeScriptSource(capability.Driver, false); err != nil {
			return nil, fmt.Errorf("browser host capability %s driver: %w", capability.ID, err)
		}
	}
	return capabilities, nil
}

func directCodingBrowserRuntimeCapabilities() ([]directCodingRuntimeCapability, error) {
	registered, err := registeredDirectCodingBrowserHostCapabilities()
	if err != nil {
		return nil, err
	}
	candidates := make([]directCodingRuntimeCapability, len(registered))
	for index, capability := range registered {
		candidates[index] = directCodingRuntimeCapability{
			ID: capability.ID, Purpose: capability.Purpose,
		}
	}
	if err := validateDirectCodingRuntimeCapabilityRegistry(candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

func directCodingBrowserHostCapabilityBlocks(
	selected []directCodingBrowserHostCapability,
) []assemblyline.SourceBlock {
	blocks := make([]assemblyline.SourceBlock, 0, len(selected))
	for _, capability := range selected {
		blocks = append(blocks, assemblyline.SourceBlock{
			ID: capability.ID,
			Static: strings.Join([]string{
				capability.Source,
				capability.Driver,
			}, "\n\n"),
			API:       capability.API,
			DependsOn: []string{"runtime.host_bridge"},
		})
	}
	return blocks
}

func directCodingBrowserHostCallsForBlock(
	block assemblyline.SourceBlock,
) ([]string, error) {
	registered, err := registeredDirectCodingBrowserHostCapabilities()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]directCodingBrowserHostCapability, len(registered))
	for _, capability := range registered {
		byID[capability.ID] = capability
	}
	calls := make([]string, 0)
	for _, capabilityID := range block.Capabilities {
		capability, exists := byID[capabilityID]
		if !exists {
			if strings.HasPrefix(capabilityID, "runtime.browser.") {
				return nil, fmt.Errorf(
					"block %s names unregistered browser host capability %s",
					block.ID, capabilityID,
				)
			}
			continue
		}
		calls = append(calls, capability.CallNames...)
	}
	return calls, nil
}

func directCodingBrowserAcceptanceForbiddenHostIdentifiers() ([]string, error) {
	registered, err := registeredDirectCodingBrowserHostCapabilities()
	if err != nil {
		return nil, err
	}
	forbiddenSet := make(map[string]struct{}, len(directCodingBrowserForbiddenRuntimeHostIdentifiers))
	for identifier := range directCodingBrowserForbiddenRuntimeHostIdentifiers {
		if identifier == "screen" {
			// The acceptance adapter binds this exact name to Testing Library and
			// separately restricts every allowed query rooted at that binding.
			continue
		}
		forbiddenSet[identifier] = struct{}{}
	}
	forbiddenSet["observeBrowserHostRequestReceipts"] = struct{}{}
	for _, capability := range registered {
		for _, callName := range capability.CallNames {
			forbiddenSet[callName] = struct{}{}
		}
	}
	forbidden := make([]string, 0, len(forbiddenSet))
	for identifier := range forbiddenSet {
		forbidden = append(forbidden, identifier)
	}
	sort.Strings(forbidden)
	return forbidden, nil
}
