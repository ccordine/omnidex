# Historical, Non-Normative Design Material

This transcript preserves an earlier specialist design discussion for provenance only. It is not an authoritative Omnidex architecture contract. Any agent, planner, tool-choice, retry-feedback, or model-authored configuration language below is superseded by the code-owned cognition and exact semantic-leaf boundaries in `AGENTS.md` and the normative documents named there.

Now that we're actually being successful doing this, at least in some degree, and much more than before, then I think perhaps my old specialist concept could work too, if it properly abandons the old concept, and instead builds it Charmander first, so like, lets say we need to do a test with chromium and stuff so that it can run a visualizer/ screen capture / etc, we could orchestrate that entire workflow in code, and like how we have the agnostic planner, we have an agnostic specialist, it gets the minimum criteria needed, and feedback / notes for when it's unsuccessful or whatever to try again, and it sets up whatever config we would need to spin up this process, for example, this means that it would be like, okay, we're doing a browser test, we'll load the page for this long, and close out, okay I got some notes back saying that wasn't long enough, and that I need to set up some interactions, and it basically returns a config from just the bare minimum context needed to operate this tool

Yes. I think the specialist concept becomes viable again once “specialist” stops meaning “an LLM agent with specialized tools.”

The Charmander-first version is more like:

A specialist is a code-owned execution machine for one class of work, with a tiny model-facing configuration boundary.

That is very different.

Your browser example is a good one because the model doesn’t need Chromium access at all. The browser specialist owns Chromium.

OBJECTIVE
"Prove the dashboard chart actually renders correctly."
        ↓
GENERAL PLANNER
creates requirement:
    rendered dashboard must be observed
    chart must become visible
    capture evidence
        ↓
SPECIALIST ROUTER
recognizes capability:
    browser_observation
        ↓
BROWSER SPECIALIST
code owns:
    Chromium lifecycle
    navigation
    timeouts
    DOM inspection
    screenshots
    video
    console logs
    network logs
    interaction execution
    cleanup

Then there may be a tiny unresolved configuration question:

Need browser run configuration.
Known:
URL = http://localhost:3000/dashboard
Acceptance = chart visible after application settles
Current known interactions = none
Return:
{
  wait_strategy,
  max_duration,
  capture_points
}

Maybe the model returns:

{
  "wait_strategy": "network_idle",
  "max_duration_ms": 5000,
  "capture_points": ["settled"]
}

Code validates that against the browser specialist’s schema and executes it.

No model is sitting there going:

click browser
wait
screenshot
inspect
maybe wait again

The browser system runs the experiment.

⸻

And what you’re describing with retries is where this gets particularly interesting.

Suppose attempt one produces:

ATTEMPT 1
navigation: success
network idle: reached
chart visible: false
capture: completed
observations:
- chart container exists
- loading indicator still present
- websocket still active
- screenshot taken at 5.0 sec
acceptance: FAILED

Code should first determine whatever it can.

Maybe it knows:

loading indicator present
→ observation occurred too early

If the browser specialist has a deterministic rule for that, it retries automatically:

wait_until("#loading-indicator", absent)

Zero inference.

But perhaps there are several plausible reasons and the specialist can’t determine the appropriate experiment.

Now you create a tiny config gap:

Previous browser attempt failed.
Required:
observe final chart state
Observed:
chart container exists
loading indicator still present after 5 seconds
application uses ongoing websocket connection
Current wait:
network_idle
Choose next bounded wait strategy:
C1: wait for loading indicator to disappear
C2: wait 15 seconds
C3: wait for chart canvas to appear

Model:

{"candidate_id":"C1"}

Code turns C1 into the actual Chromium/Playwright configuration and reruns it.

That’s much closer to the thing you’ve been trying to build everywhere else.

⸻

The generic specialist architecture could be extremely small:

type Specialist interface {
    Capability() Capability
    Schema() ConfigSchema
    DeriveConfig(State) (Config, MissingConfigFacts)
    ValidateConfig(Config) error
    Execute(context.Context, Config) AttemptResult
    Evaluate(AttemptResult, AcceptanceCriteria) Evaluation
    ReduceFailure(Evaluation) []Fact
}

Notice what’s absent:

Think()
ChooseTool()
Act()
Reflect()
Remember()

None of that agent shit.

A specialist instead has:

CAPABILITY
CONFIGURATION
RUNNER
OBSERVATION
VERIFIER
FAILURE REDUCER

And optionally one or more tiny semantic configuration stations.

⸻

The general workflow

I think it becomes something like this:

OBJECTIVE
    ↓
capability requirement derived
    ↓
SPECIALIST REGISTRY
    ↓
matching specialist selected by code
    ↓
derive all deterministic configuration
    ↓
configuration complete?
    │
    ├── yes
    │    ↓
    │  execute
    │
    └── no
         ↓
      named config gap
         ↓
      tiny model station
         ↓
      bounded config fragment
         ↓
      code validates
         ↓
       execute
         ↓
      collect evidence
         ↓
      code evaluates
         ↓
      success?
      ├── yes → artifact/facts → objective complete
      └── no
           ↓
        reduce failure
           ↓
        deterministic correction?
        ├── yes → retry
        └── no → next tiny config gap

And that’s agnostic.

The browser specialist is merely one implementation.

Another could be:

CompilerSpecialist
DatabaseSpecialist
HTTPIntegrationSpecialist
ContainerSpecialist
PerformanceSpecialist
VisualRegressionSpecialist
MobileDeviceSpecialist
NetworkSpecialist
GameSimulationSpecialist

But those aren’t model personas.

They’re programmable experimental apparatuses.

⸻

The model should probably return configuration fragments, not whole specialist programs

This is another place I would keep the responsibility tiny.

For the browser case, avoid:

{
  "steps": [
    {"action":"navigate", ...},
    {"action":"wait", ...},
    {"action":"click", ...},
    {"action":"screenshot", ...}
  ]
}

because we’re halfway back to tool calling.

Better:

{
  "settle_condition": "element_absent",
  "selector_ref": "S17"
}

where code already knows:

browser test
→ navigate
→ establish environment
→ settle
→ perform required fixture interactions
→ observe
→ capture
→ verify
→ cleanup

The LLM configures a parameter in the known workflow.

For interactions, same idea.

Suppose the objective actually requires:

Open the menu, select Analytics, then verify chart.

Code may already derive those interactions from a test fixture, DOM structure, or explicit acceptance definition.

If it can’t map “Analytics” to one of several controls:

Which element corresponds to the requested Analytics view?
E17: button "Analytics"
E18: link "/analytics"
E19: div aria-label="analytics-chart"

Model:

{"candidate_id":"E17"}

Code generates the click.

Again, the model never clicks.

⸻

And feedback becomes particularly clean

Instead of giving the model a giant transcript of everything that happened:

Attempt 1...
Attempt 2...
Attempt 3...
full console...
full DOM...
screenshots...

the specialist feeds the normal context reduction pipeline.

So perhaps the machine reduces all of that to:

Objective:
observe populated chart
Attempt:
chart canvas appeared
canvas remained blank
Relevant observations:
- API /metrics returned 200
- response contained 47 datapoints
- console error: "ChartAdapter undefined"
- screenshot region R17 blank
Unresolved question:
which evidence should determine the next diagnostic configuration?

Tiny question, tiny answer, new experiment.

Every retry gets another clean desk.

That means these specialists inherit the same architecture you’ve finally gotten working in cognition.

⸻

And this makes the old specialist idea much more powerful because specialists can become deeply sophisticated without the models becoming sophisticated.

Your Chromium specialist can eventually know how to:

* provision browser profiles;
* run Chromium versions;
* launch Xvfb/headless/headed sessions;
* capture screenshots/video;
* inspect DOM/accessibility trees;
* record console/network;
* inject test fixtures;
* emulate viewports;
* compare images;
* monitor CPU/memory;
* execute deterministic interactions;
* detect page stability;
* isolate test runs;
* archive evidence;
* clean everything afterward.

The LLM doesn’t have to know how any of that works.

It might occasionally provide:

selector interpretation
candidate relevance
interaction interpretation
wait condition choice
visual semantic judgment
diagnostic hypothesis ID

That’s the 10%.

The other 90% is an increasingly monstrous browser-testing machine.

And importantly, the general planner doesn’t need to know Chromium either.

It just knows:

Objective O17 requires:
    Capability: rendered_ui_observation
Acceptance:
    chart visible
    populated
    no error state

The specialist registry says:

rendered_ui_observation
→ BrowserSpecialist

Then the browser specialist takes over its bounded workflow and eventually returns:

FACT:
chart.visible = true
FACT:
chart.populated = true
ARTIFACT:
screenshot://A71
EVIDENCE:
browser-run://R31

Those go back into the general cognition/workload machine.

So you wind up with:

                     OMNIDEX
                        │
                 objective graph
                        │
             "need fact/capability X"
                        │
                 specialist registry
                        │
       ┌────────────────┼────────────────┐
       ▼                ▼                ▼
   Browser           Compiler        Database
 Specialist         Specialist      Specialist
       │                │                │
code workflow      code workflow    code workflow
       │                │                │
 tiny gaps          tiny gaps        tiny gaps
       │                │                │
      LLM              LLM              LLM
       │                │                │
       └────────────────┼────────────────┘
                        ▼
                  validated facts
                        ↓
                  objective resumes

And then Charizard eventually has a very obvious role here too.

If the browser specialist repeatedly discovers that React dashboards need:

wait until loading indicator disappears
then wait one animation frame
then capture

you shouldn’t keep spending inference discovering that.

Promote the successful configuration pattern into a deterministic specialist strategy:

ReactDashboardSettledStrategy

Now future runs go straight through code.

So specialists can actually consume semantic uncertainty over time.

That is much better than the old specialist-agent concept.

The old idea was effectively:

Give a model expertise and tools.

The Charmander version is:

Build expertise into a machine, and use models only to parameterize the parts of the machine that cannot yet be determined exactly.

That fits the architecture you’re proving now almost perfectly.
