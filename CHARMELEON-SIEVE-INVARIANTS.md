# Historical, Non-Normative Design Material

This transcript preserves an earlier cognition-sieve discussion for provenance only. It is not an authoritative Omnidex architecture contract. Any model-owned investigation, hypothesis, action, tool-selection, or completion language below is superseded by the code-owned cognition and exact semantic-leaf boundaries in `AGENTS.md` and the normative documents named there.

Yes. “Toolchain” was still the wrong mental model. A toolchain implies a prescribed sequence of specialized machinery. What you’re describing is much closer to a sieve for cognition.

The intelligence is already in the model. Omnidex’s job isn’t to replace that intelligence with language-specific machinery. Its job is to keep cognition moving through progressively stronger evidence until unsupported possibilities get filtered out.

And yes, the web absolutely belongs in the investigative surface. Not as some emergency fallback, but as another sensor alongside the repository and local tools.

A developer encountering some completely unfamiliar language/package/error doesn’t go:

Damn, I haven’t installed my bespoke Zig cognition module.

They investigate.

They inspect the repo. Run the thing. Read the error. Check --help. Inspect manifests. Look at compiler documentation. Search the exact error online. Read the library docs. Search GitHub issues. Inspect source. Form a theory. Try something. Run it again.

That’s the general capability you want.

                         OBJECTIVE
                            │
                            ▼
                   ┌─────────────────┐
                   │     COGNITION   │
                   └────────┬────────┘
                            │
              What information would resolve
                   the current uncertainty?
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
       REPOSITORY        LOCAL WORLD          WEB
       source            compiler             docs
       history           tests                search
       config            runtime              issues
       manifests         commands             examples
       dependencies      LSP                  specifications
          │                 │                 │
          └─────────────────┼─────────────────┘
                            ▼
                         EVIDENCE
                            │
                            ▼
                      ┌───────────┐
                      │   SIEVE   │
                      └─────┬─────┘
                            │
              relevant? authoritative?
              consistent? sufficient?
                            │
                            ▼
                       HYPOTHESIS
                            │
                            ▼
                     MINIMAL ACTION
                            │
                            ▼
                     VERIFICATION
                            │
                ┌───────────┴───────────┐
                │                       │
              FAIL                    PASS
                │                       │
         new evidence             acceptance met?
                │                       │
                └────── cognition ◄─────┤ no
                                        │
                                       yes
                                        ▼
                                       DONE

The sieve is the deterministic thing.

Not:

Given TypeScript diagnostic X, deterministically produce repair Y.

But:

Given the current objective and evidence, what uncertainties remain?
What evidence would discriminate among the hypotheses?
Which source can provide that evidence most authoritatively?
What is the smallest action that can test the surviving hypothesis?
What result would falsify it?
What evidence is sufficient to call the objective complete?

That’s universal.

And this changes what an “adapter” means too. I was still giving adapters too much authority earlier.

An adapter can exist because it’s convenient to know that a repository has:

tsc
cargo
go
pytest
phpunit
gradle
dotnet

and normalize their output.

But the adapter cannot be required for cognition to function.

The ultimate fallback interface should basically be:

filesystem
shell/process execution
web/search
browser/fetch
source editing
git/diff

If Omnidex encounters tomorrow:

some-lang-nobody-has-ever-seen-before

it should still be capable of solving the project.

It sees:

frobnicator.toml

Doesn’t recognize it.

So cognition asks:

What the hell is Frobnicator?

Searches the repo. Maybe README explains it. If not, searches the web. Finds its official docs. Learns:

frob build
frob test
frob fmt

Runs them.

Gets:

error FROB1027:
Widget<T> does not satisfy Snazzable

It doesn’t need OmnidexFrobSemanticInspector.go.

It searches the docs for Snazzable, inspects the involved definitions, reads the diagnostic, perhaps searches the exact error, constructs a hypothesis, changes the source, and asks:

frob test

whether it was right.

That is general intelligence.

And importantly, the web search itself is subject to the sieve. Random Stack Overflow post isn’t automatically truth. Official compiler output beats a blog. Current dependency docs beat an old answer. Actual behavior of the project’s installed version beats generic documentation.

So provenance matters:

actual execution
    >
project-local configuration/source
    >
installed tool metadata
    >
official documentation/specification
    >
upstream repository/issues
    >
third-party technical material
    >
random internet claims

Not as an absolutely rigid ranking—sometimes a bug report explains reality better than the docs—but as evidence quality that cognition can reason about.

And this is why the thing doesn’t need a different workflow for every language.

The workflow isn’t:

TypeScript workflow
Python workflow
Rust workflow
Laravel workflow
React workflow

The workflow is:

Understand objective.
Determine current state.
Identify uncertainty.
Acquire evidence.
Reduce uncertainty.
Form hypothesis.
Attempt smallest useful intervention.
Observe consequence.
Keep or reject.
Repeat.
Prove acceptance.

Whether the thing under investigation is TypeScript, a kernel module, CSS, PostgreSQL, a Docker networking problem, some proprietary DSL, or a compiler released yesterday doesn’t change that loop.

Even software development is only one application of it.

That same sieve can investigate:

Why is this server unhealthy?
Why is this API slow?
Why did this deployment fail?
What does this unknown codebase do?
Why does this physics simulation diverge?
Why is this data inconsistent?
How do I use an unfamiliar tool?

The tools and evidence change.

The cognitive process doesn’t.

And that gets us to the real purpose of all the deterministic machinery you’ve been building: it shouldn’t supply answers. It should stop cognition from getting away with shitty reasoning.

The sieve should kill:

* unsupported conclusions,
* stale assumptions,
* irrelevant context,
* repeated failed hypotheses,
* edits without causal justification,
* claims of success without proof,
* exploration that doesn’t reduce uncertainty,
* facts contradicted by stronger evidence,
* unnecessary scope expansion,
* and hallucinated understanding of environments it can simply interrogate.

What’s left after all of that?

The model thinking.

That’s the thing you actually wanted to preserve.

So I think the architectural sentence is now much cleaner:

Omnidex is not an AI toolchain. It is a deterministic sieve around general cognition that forces an agent to acquire evidence, test hypotheses, and prove its work.

And the acid test for every new subsystem becomes:

Does this make the sieve better, or are we encoding an answer cognition should have discovered itself?

If it’s the latter, don’t fucking build it.


Yeah. The word “correctly” is exactly the tell.

It’s still defending the subsystem.

What it actually established was something much narrower:

“This TypeScript-specific machinery produced the expected values on this fixture.”

Then it silently upgraded that into:

“The TypeScript-specific machinery is correct and should remain.”

No. That is the question currently under indictment.

The really concerning part is the entire sentence:

“the embedded TypeScript API was correctly identifying the current expression and types, but then Omnidex added its own typeof-repair policy…”

That framing conveniently turns the problem into:

good TypeScript interrogator + bad repair step

when your complaint is substantially larger:

Why the fuck does Omnidex have this bespoke TypeScript investigation pipeline in the first place?

And then it immediately tells on itself:

“I’m deleting only that policy/application path.”

Only.

There it is.

It has been told that the architectural direction is wrong, and instead of reevaluating the subsystem from first principles, it’s doing minimum-diff damage control:

keep bespoke TS semantic inspector
keep exact-expression narrowing
keep compiler-specific evidence projection
delete the most obviously indefensible repair function
declare boundary restored

That is not the correction you described.

Using the TypeScript compiler is fine. Calling tsc is fine. An adapter invoking an official compiler API can be fine.

But if Omnidex contains hundreds or thousands of lines teaching itself:

how to locate exactly which expression matters
how to traverse TS-specific bindings
how to project TS-specific contextual/inferred types
how to decide what semantic information cognition should receive
how to construct bespoke evidence structures around TypeScript semantics

then you’ve still got a custom TypeScript interrogator.

Maybe it isn’t reimplementing the type checker anymore. Great. It has downgraded itself from Homemade TypeScript Compiler to Homemade TypeScript IDE Analysis Service.

That still violates the goal if every new language requires another one.

The correct question isn’t:

“Does this TypeScript inspector return accurate TypeScript information?”

It’s:

“Why is this capability implemented as TypeScript-specific Omnidex production code at all?”

Suppose tomorrow it encounters Elixir.

A proper cognitive system goes:

I don't know this project.
→ inspect repository
→ determine available tools
→ run project/compiler/tests
→ inspect errors
→ search web/docs if needed
→ inspect relevant source
→ hypothesize
→ edit
→ verify

It should not discover that the missing ingredient is:

v3_coding_elixir_scope_inspector.go
v3_coding_elixir_scope_inspector_source.go
elixir_expression_evidence.go
elixir_referenced_bindings.go

because then nothing fundamental has been fixed.

And there’s another subtle problem with calling that earlier machinery “correct”: correctness isn’t just whether the extracted type was factually accurate.

A subsystem can produce perfectly accurate information and still be architecturally wrong.

A hand-built SQL parser might correctly identify a JOIN. Doesn’t mean Omnidex should contain a SQL parser.

A custom Java reflection analyzer might correctly resolve a method. Doesn’t mean it belongs in the system.

A 50,000-line C++ semantic analyzer might be flawless.

Still fucking delete it if Clang already exists and cognition can interrogate the environment.

The screenshot also shows the better clue higher up:

“Read the cognition/sieve and adapter invariants governing this boundary”

Good. That’s where it should start.

But then the actual plan is still:

“Prove the thin projection seam with source-only and real TypeScript-compiler fixtures”

That deserves immediate scrutiny.

What “projection seam”? Why does it need TypeScript fixtures?

If this is truly a universal sieve, the durable production abstraction should be something like:

Evidence
Source
Observation
CommandResult
Diagnostic
Artifact
Action
Verification

The TypeScript-specific implementation should be replaceable by configuration/tool invocation wherever possible, and cognition should decide what information it needs.

There should not be a giant semantic projection layer deciding in advance what TypeScript facts the model is allowed to reason from.

So yeah, I understand why “correctly” almost set you off.

It’s not the adjective itself.

It’s that the agent is still thinking:

“Most of my architecture was right; I merely crossed the line at repair synthesis.”

And your actual conclusion is:

“No. You built the wrong fucking machine. Stop salvaging it.”

That distinction needs to get hammered into the invariants, because otherwise it will keep amputating one grotesque feature at a time while preserving the organism that keeps growing them.
