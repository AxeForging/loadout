import { useMemo, useState } from "react";
import type { CSSProperties } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Answers, type WizardQuestion, type WizardSection } from "../api";
import { PageHeader } from "../components/Layout";
import { InstructionListEditor } from "../components/InstructionListEditor";

// The builder renders the wizard schema served by the binary — the exact
// same questionnaire as `loadout wizard`, with a review step before any
// store write. Laid out as an assembly line: rail of stations, one active.
export function BuilderPage() {
  const schema = useQuery({ queryKey: ["wizard-schema"], queryFn: api.wizardSchema });
  const presets = useQuery({ queryKey: ["wizard-presets"], queryFn: api.wizardPresets });
  const profiles = useQuery({ queryKey: ["profiles"], queryFn: api.profiles });

  const [answers, setAnswers] = useState<Answers>({});
  const [preset, setPreset] = useState<string | undefined>(undefined);
  const [profile, setProfile] = useState<string | undefined>(undefined);
  const [step, setStep] = useState(0);

  const queryClient = useQueryClient();
  const plan = useMutation({ mutationFn: () => api.wizardPlan(answers, preset) });
  const generate = useMutation({
    mutationFn: () => api.wizardGenerate(answers, preset),
    onSuccess: () => void queryClient.invalidateQueries(),
  });
  const loadProfile = useMutation({
    mutationFn: (name: string) => api.profile(name),
    onSuccess: (loaded, name) => {
      setAnswers(loaded);
      setProfile(name);
      setPreset(undefined);
    },
  });

  const sections = schema.data ?? [];
  const atReview = step === sections.length;

  const set = (id: string, value: unknown) => setAnswers((a) => ({ ...a, [id]: value }));

  const goReview = () => {
    setStep(sections.length);
    plan.mutate();
  };

  const answeredIn = (s: WizardSection) =>
    s.questions.filter((q) => answers[q.id] !== undefined).length;

  if (schema.isLoading) return <p className="text-sm text-ink-faint">Loading…</p>;

  return (
    <div>
      <PageHeader
        crate="armory / builder"
        title="Build your kit"
        sub="Answer once — loadout generates conventions, testing standards, SDD commands and continuity artifacts for every agent. Nothing is written until you confirm the review."
      />

      <div className="rise mb-3 flex flex-wrap items-center gap-x-3 gap-y-2">
        <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
          start from
        </span>
        {(presets.data ?? []).map((p) => (
          <button
            key={p}
            onClick={() => {
              setPreset(preset === p ? undefined : p);
              setProfile(undefined);
            }}
            data-on={preset === p}
            className="chip font-mono text-xs"
          >
            {p}
          </button>
        ))}
        <span className="text-xs text-ink-faint">your answers below override the preset</span>
      </div>

      {(profiles.data ?? []).length > 0 ? (
        <div className="rise mb-10 flex flex-wrap items-center gap-x-3 gap-y-2" style={{ "--i": 1 } as CSSProperties}>
          <span className="font-mono text-[10px] uppercase tracking-[0.18em] text-ink-faint">
            or a saved profile
          </span>
          {(profiles.data ?? []).map((p) => (
            <button
              key={p}
              onClick={() => loadProfile.mutate(p)}
              data-on={profile === p}
              className="chip font-mono text-xs"
            >
              {p}
            </button>
          ))}
          <span className="text-xs text-ink-faint">
            loads that team's or org's answers — one org, one project type, one profile
          </span>
        </div>
      ) : (
        <div className="mb-10" />
      )}

      <div className="flex gap-12">
        {/* Assembly rail: numbered stations joined by a line, review at the end. */}
        <ol className="relative w-48 shrink-0">
          <span aria-hidden className="absolute bottom-4 left-[11px] top-4 w-px bg-line" />
          {sections.map((s, i) => {
            const on = i === step && !atReview;
            const done = answeredIn(s);
            return (
              <li key={s.id} className="relative">
                <button
                  onClick={() => setStep(i)}
                  className={`group flex w-full items-center gap-3.5 py-2 text-left text-sm transition-colors ${
                    on ? "text-ink" : "text-ink-dim hover:text-ink"
                  }`}
                >
                  <span
                    className={`z-[1] flex h-6 w-6 flex-none items-center justify-center border font-mono text-[10px] tnum transition-colors ${
                      on
                        ? "border-ember bg-ember-faint text-ember-bright"
                        : done > 0
                          ? "border-line-strong bg-raised text-ink-dim"
                          : "border-line bg-bg text-ink-faint"
                    }`}
                  >
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className={`block truncate ${on ? "font-medium" : ""}`}>
                      {s.title.split("—")[0].trim()}
                    </span>
                    <span className="block font-mono text-[10px] text-ink-faint tnum">
                      {done}/{s.questions.length} answered
                    </span>
                  </span>
                </button>
              </li>
            );
          })}
          <li className="relative">
            <button
              onClick={goReview}
              className={`group flex w-full items-center gap-3.5 py-2 text-left text-sm transition-colors ${
                atReview ? "text-ember-bright" : "text-ember hover:text-ember-bright"
              }`}
            >
              <span
                className={`z-[1] flex h-6 w-6 flex-none items-center justify-center border font-mono text-xs transition-colors ${
                  atReview
                    ? "border-ember bg-ember text-bg-deep"
                    : "border-ember-dim bg-bg text-ember"
                }`}
              >
                ▸
              </span>
              <span className={atReview ? "font-medium" : ""}>Review</span>
            </button>
          </li>
        </ol>

        <div className="min-w-0 flex-1">
          {atReview ? (
            <ReviewStep
              plan={plan}
              generate={generate}
              answers={answers}
              onBack={() => setStep(Math.max(0, sections.length - 1))}
            />
          ) : sections[step] ? (
            <SectionForm
              key={sections[step].id}
              section={sections[step]}
              answers={answers}
              set={set}
              onNext={() => (step + 1 < sections.length ? setStep(step + 1) : goReview())}
              isLast={step + 1 >= sections.length}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}

function SectionForm({
  section,
  answers,
  set,
  onNext,
  isLast,
}: {
  section: WizardSection;
  answers: Answers;
  set: (id: string, v: unknown) => void;
  onNext: () => void;
  isLast: boolean;
}) {
  return (
    <div className="page">
      <h2 className="font-display text-lg font-semibold text-ink">{section.title}</h2>
      <div className="mt-7 space-y-8">
        {section.questions.map((q, i) => (
          <div key={q.id} className="rise" style={{ "--i": i + 1 } as CSSProperties}>
            <Question q={q} value={answers[q.id]} onChange={(v) => set(q.id, v)} />
          </div>
        ))}
      </div>
      <button onClick={onNext} className="btn-outline mt-10">
        {isLast ? "Go to review" : "Next section"} <span aria-hidden>→</span>
      </button>
    </div>
  );
}

function Question({
  q,
  value,
  onChange,
}: {
  q: WizardQuestion;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const current = value ?? q.default;

  const selected = useMemo(() => {
    if (q.type !== "multi") return new Set<string>();
    return new Set(Array.isArray(current) ? (current as string[]) : []);
  }, [q.type, current]);

  return (
    <fieldset>
      <legend className="text-sm font-medium text-ink">{q.prompt}</legend>
      <div className="mt-3">
        {q.type === "bool" ? (
          <div className="flex gap-2">
            {[true, false].map((v) => (
              <button
                key={String(v)}
                onClick={() => onChange(v)}
                data-on={current === v}
                className="chip"
              >
                {v ? "yes" : "no"}
              </button>
            ))}
          </div>
        ) : q.type === "text" ? (
          <input
            type="text"
            value={typeof current === "string" ? current : ""}
            onChange={(e) => onChange(e.target.value)}
            placeholder="free text"
            className="field w-full max-w-md"
          />
        ) : q.type === "list" ? (
          <InstructionListEditor
            value={Array.isArray(current) ? (current as string[]) : []}
            onChange={onChange}
          />
        ) : (
          <div className="flex flex-wrap gap-2">
            {(q.options ?? []).map((o) => {
              const active = q.type === "select" ? current === o.value : selected.has(o.value);
              return (
                <button
                  key={o.value}
                  onClick={() => {
                    if (q.type === "select") {
                      onChange(o.value);
                    } else {
                      const next = new Set(selected);
                      if (next.has(o.value)) next.delete(o.value);
                      else next.add(o.value);
                      onChange([...next]);
                    }
                  }}
                  data-on={active}
                  className="chip"
                >
                  {active ? (
                    <span aria-hidden className="mr-1.5 font-mono text-[9px] align-middle">
                      ■
                    </span>
                  ) : null}
                  {o.label}
                </button>
              );
            })}
          </div>
        )}
      </div>
    </fieldset>
  );
}

function ReviewStep({
  plan,
  generate,
  answers,
  onBack,
}: {
  plan: ReturnType<typeof useMutation<{ create: string[]; skip: string[] }, Error, void>>;
  generate: ReturnType<typeof useMutation<{ created: string[]; skipped: string[] }, Error, void>>;
  answers: Answers;
  onBack: () => void;
}) {
  const queryClient = useQueryClient();
  const [profileName, setProfileName] = useState("");
  const saveProfile = useMutation({
    mutationFn: (name: string) => api.saveProfile(name, answers),
    onSuccess: () => {
      setProfileName("");
      void queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });

  if (generate.isSuccess) {
    return (
      <div className="page max-w-[65ch]">
        <h2 className="stamp font-display text-lg font-semibold text-sync">
          ■ {generate.data.created.length} artifacts in the store
        </h2>
        <ul className="mt-3 space-y-1">
          {generate.data.created.map((c, i) => (
            <li key={c} className="rise font-mono text-sm text-ink-dim" style={{ "--i": i } as CSSProperties}>
              <span className="mr-2 text-sync">+</span>
              {c}
            </li>
          ))}
        </ul>
        <p className="mt-5 text-sm leading-relaxed text-ink-dim">
          They are not installed anywhere yet — the pending-changes bar below will offer the gated
          sync, or review each file under Store first.
        </p>
      </div>
    );
  }

  return (
    <div className="page max-w-[65ch]">
      <h2 className="font-display text-lg font-semibold text-ink">Review</h2>
      {plan.isPending ? (
        <p className="mt-3 text-sm text-ink-faint">Planning…</p>
      ) : plan.isError ? (
        <p className="mt-3 text-sm text-danger">{String(plan.error)}</p>
      ) : plan.data ? (
        <>
          <p className="mt-3 text-sm text-ink-dim">
            The builder will create{" "}
            <span className="tnum font-medium text-ink">{plan.data.create.length}</span> artifact
            {plan.data.create.length === 1 ? "" : "s"} in the store:
          </p>
          {plan.data.create.length > 0 ? (
            <ul className="mt-3 space-y-1 border-y border-line py-3">
              {plan.data.create.map((c, i) => (
                <li key={c} className="rise font-mono text-sm text-ink" style={{ "--i": i } as CSSProperties}>
                  <span className="mr-2 text-sync">+</span>
                  {c}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-2 font-mono text-sm text-ink-faint">
              nothing — everything already exists
            </p>
          )}
          {plan.data.skip.length > 0 ? (
            <p className="mt-3 font-mono text-xs text-ink-faint">
              kept untouched (already exist): {plan.data.skip.join(" · ")}
            </p>
          ) : null}
          <div className="mt-7 flex items-center gap-3">
            <button
              onClick={() => generate.mutate()}
              disabled={generate.isPending || plan.data.create.length === 0}
              className="btn-ember"
            >
              {generate.isPending ? "Writing…" : `Write ${plan.data.create.length} to store`}
            </button>
            <button
              onClick={onBack}
              className="px-2 py-1.5 text-sm text-ink-dim transition-colors hover:text-ink"
            >
              Back
            </button>
            {generate.isError ? (
              <span className="text-xs text-danger">{String(generate.error)}</span>
            ) : null}
          </div>

          <div className="mt-10 border-t border-line pt-5">
            <h3 className="crate">save these answers as a profile</h3>
            <p className="mt-2 text-xs text-ink-faint">
              Reuse this exact stack + conventions next time — for another repo, or share it with
              your team.
            </p>
            <div className="mt-3 flex items-center gap-2">
              <input
                type="text"
                value={profileName}
                onChange={(e) => setProfileName(e.target.value)}
                placeholder="acme-frontend"
                className="field w-48"
              />
              <button
                onClick={() => saveProfile.mutate(profileName)}
                disabled={saveProfile.isPending || profileName.trim() === ""}
                className="btn-ghost"
              >
                {saveProfile.isPending ? "Saving…" : "Save as profile"}
              </button>
              {saveProfile.isSuccess ? (
                <span className="stamp text-xs text-sync">■ saved</span>
              ) : saveProfile.isError ? (
                <span className="text-xs text-danger">{String(saveProfile.error)}</span>
              ) : null}
            </div>
          </div>
        </>
      ) : null}
    </div>
  );
}
