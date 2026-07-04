import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Answers, type WizardQuestion, type WizardSection } from "../api";
import { PageHeader } from "../components/Layout";

// The builder renders the wizard schema served by the binary — the exact
// same questionnaire as `loadout wizard`, with a review step before any
// store write.
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

  if (schema.isLoading) return <p className="text-sm text-ink-faint">Loading…</p>;

  return (
    <div>
      <PageHeader
        crate="armory / builder"
        title="Build your kit"
        sub="Answer once — loadout generates conventions, testing standards, SDD commands and continuity artifacts for every agent. Nothing is written until you confirm the review."
      />

      <div className="mb-3 flex items-center gap-3">
        <span className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">
          start from
        </span>
        {(presets.data ?? []).map((p) => (
          <button
            key={p}
            onClick={() => {
              setPreset(preset === p ? undefined : p);
              setProfile(undefined);
            }}
            className={
              preset === p
                ? "rounded-sm bg-ember-faint px-3 py-1 font-mono text-xs text-ember-bright"
                : "rounded-sm border border-line px-3 py-1 font-mono text-xs text-ink-dim hover:text-ink"
            }
          >
            {p}
          </button>
        ))}
        <span className="text-xs text-ink-faint">your answers below override the preset</span>
      </div>

      {(profiles.data ?? []).length > 0 ? (
        <div className="mb-8 flex items-center gap-3">
          <span className="font-mono text-[11px] uppercase tracking-[0.2em] text-ink-faint">
            or a saved profile
          </span>
          {(profiles.data ?? []).map((p) => (
            <button
              key={p}
              onClick={() => loadProfile.mutate(p)}
              className={
                profile === p
                  ? "rounded-sm bg-ember-faint px-3 py-1 font-mono text-xs text-ember-bright"
                  : "rounded-sm border border-line px-3 py-1 font-mono text-xs text-ink-dim hover:text-ink"
              }
            >
              {p}
            </button>
          ))}
          <span className="text-xs text-ink-faint">
            loads that team's or org's answers — one org, one project type, one profile
          </span>
        </div>
      ) : (
        <div className="mb-8" />
      )}

      <div className="flex gap-10">
        <ol className="w-44 shrink-0 space-y-1">
          {sections.map((s, i) => (
            <li key={s.id}>
              <button
                onClick={() => setStep(i)}
                className={`w-full rounded-sm px-3 py-1.5 text-left text-sm transition-colors ${
                  i === step && !atReview
                    ? "bg-surface font-medium text-ink"
                    : "text-ink-dim hover:text-ink"
                }`}
              >
                <span className="mr-2 font-mono text-[11px] text-ink-faint">
                  {String(i + 1).padStart(2, "0")}
                </span>
                {s.title.split("—")[0].trim()}
              </button>
            </li>
          ))}
          <li>
            <button
              onClick={goReview}
              className={`w-full rounded-sm px-3 py-1.5 text-left text-sm ${
                atReview ? "bg-surface font-medium text-ember" : "text-ember hover:text-ember-bright"
              }`}
            >
              <span className="mr-2 font-mono text-[11px]">→</span>Review
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
    <div className="rise">
      <h2 className="font-display text-lg font-semibold text-ink">{section.title}</h2>
      <div className="mt-6 space-y-7">
        {section.questions.map((q) => (
          <Question key={q.id} q={q} value={answers[q.id]} onChange={(v) => set(q.id, v)} />
        ))}
      </div>
      <button
        onClick={onNext}
        className="mt-8 rounded-sm border border-ember-dim px-4 py-1.5 font-display text-sm font-medium text-ember transition-colors hover:border-ember hover:text-ember-bright"
      >
        {isLast ? "Go to review" : "Next section"}
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
      <div className="mt-2.5">
        {q.type === "bool" ? (
          <div className="flex gap-1">
            {[true, false].map((v) => (
              <button
                key={String(v)}
                onClick={() => onChange(v)}
                className={
                  current === v
                    ? "rounded-sm bg-surface px-3.5 py-1 text-sm font-medium text-ink"
                    : "rounded-sm px-3.5 py-1 text-sm text-ink-faint hover:text-ink"
                }
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
            className="w-full max-w-md rounded-sm border border-line bg-surface px-3 py-1.5 font-mono text-sm text-ink outline-none placeholder:text-ink-faint focus:border-ember-dim"
          />
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {(q.options ?? []).map((o) => {
              const active =
                q.type === "select" ? current === o.value : selected.has(o.value);
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
                  className={
                    active
                      ? "rounded-sm bg-ember-faint px-3 py-1 text-sm text-ember-bright"
                      : "rounded-sm border border-line px-3 py-1 text-sm text-ink-dim hover:border-line-strong hover:text-ink"
                  }
                >
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
      <div className="rise max-w-[65ch]">
        <h2 className="font-display text-lg font-semibold text-sync">
          ■ {generate.data.created.length} artifacts in the store
        </h2>
        <p className="mt-2 font-mono text-sm text-ink-dim">
          {generate.data.created.join(" · ")}
        </p>
        <p className="mt-4 text-sm leading-relaxed text-ink-dim">
          They are not installed anywhere yet — the pending-changes bar below will offer the
          gated sync, or review each file under Store first.
        </p>
      </div>
    );
  }

  return (
    <div className="rise max-w-[65ch]">
      <h2 className="font-display text-lg font-semibold text-ink">Review</h2>
      {plan.isPending ? (
        <p className="mt-3 text-sm text-ink-faint">Planning…</p>
      ) : plan.isError ? (
        <p className="mt-3 text-sm text-danger">{String(plan.error)}</p>
      ) : plan.data ? (
        <>
          <p className="mt-3 text-sm text-ink-dim">
            The builder will create{" "}
            <span className="font-medium text-ink">{plan.data.create.length}</span> artifact
            {plan.data.create.length === 1 ? "" : "s"} in the store:
          </p>
          <p className="mt-2 font-mono text-sm leading-relaxed text-ink">
            {plan.data.create.join(" · ") || "nothing — everything already exists"}
          </p>
          {plan.data.skip.length > 0 ? (
            <p className="mt-2 font-mono text-xs text-ink-faint">
              kept untouched (already exist): {plan.data.skip.join(" · ")}
            </p>
          ) : null}
          <div className="mt-6 flex items-center gap-3">
            <button
              onClick={() => generate.mutate()}
              disabled={generate.isPending || plan.data.create.length === 0}
              className="rounded-sm bg-ember px-4 py-1.5 font-display text-sm font-semibold text-bg transition-colors hover:bg-ember-bright disabled:opacity-50"
            >
              {generate.isPending ? "Writing…" : `Write ${plan.data.create.length} to store`}
            </button>
            <button onClick={onBack} className="px-2 py-1.5 text-sm text-ink-dim hover:text-ink">
              Back
            </button>
            {generate.isError ? (
              <span className="text-xs text-danger">{String(generate.error)}</span>
            ) : null}
          </div>

          <div className="mt-8 border-t border-line pt-5">
            <h3 className="font-mono text-[11px] uppercase tracking-[0.15em] text-ink-faint">
              Save these answers as a profile
            </h3>
            <p className="mt-1.5 text-xs text-ink-faint">
              Reuse this exact stack + conventions next time — for another repo, or share it with
              your team.
            </p>
            <div className="mt-2.5 flex items-center gap-2">
              <input
                type="text"
                value={profileName}
                onChange={(e) => setProfileName(e.target.value)}
                placeholder="acme-frontend"
                className="w-48 rounded-sm border border-line bg-surface px-3 py-1.5 font-mono text-sm text-ink outline-none placeholder:text-ink-faint focus:border-ember-dim"
              />
              <button
                onClick={() => saveProfile.mutate(profileName)}
                disabled={saveProfile.isPending || profileName.trim() === ""}
                className="rounded-sm border border-line-strong px-3 py-1.5 font-display text-sm text-ink-dim transition-colors hover:border-ember-dim hover:text-ink disabled:opacity-50"
              >
                {saveProfile.isPending ? "Saving…" : "Save as profile"}
              </button>
              {saveProfile.isSuccess ? (
                <span className="text-xs text-sync">■ saved</span>
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
