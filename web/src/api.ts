// Typed fetch layer for the loadout API. Server state lives entirely in
// TanStack Query — no client store needed.

export type Agent = {
  id: string;
  name: string;
  installed: boolean;
  home: string;
};

export type Overview = {
  store: { name: string; root: string; artifacts: number };
  agents: Agent[];
  pending: number;
  states: Record<string, number>;
};

export type StatusRow = {
  artifact: string;
  kind: string;
  agent: string;
  state: string;
};

export type StatusResponse = {
  rows: StatusRow[];
  skips: { agent: string; reason: string }[];
};

export type Artifact = {
  id: string;
  kind: string;
  path: string;
  origin?: string;
  targets?: string[];
  tags?: string[];
};

export type ArtifactDetail = { artifact: Artifact; files: Record<string, string> };

export type Change = { op: string; agent: string; artifact: string; relPath: string };
export type SyncPlan = { token: string; changes: Change[] };

export type Diff = { agent: string; artifact: string; relPath: string; diff: string };

export type Source = { name: string; url: string; ref?: string; commit: string; subdir?: string };
export type SourceCheck = { name: string; pinned: string; remote: string; hasUpdate: boolean; error?: string };
export type SourceAddResult = { ref: Source; added: string[]; skipped: string[] };

export type WizardOption = { value: string; label: string };
export type WizardQuestion = {
  id: string;
  prompt: string;
  type: "select" | "multi" | "text" | "bool";
  options?: WizardOption[];
  default?: unknown;
};
export type WizardSection = { id: string; title: string; questions: WizardQuestion[] };
export type Answers = Record<string, unknown>;

export type BundleMeta = { name: string; createdBy: string; createdAt: string; artifacts: number };
export type BundleImportPlan = {
  token: string;
  meta: BundleMeta;
  new: Artifact[];
  conflicts: Artifact[];
  identical: string[];
  sources: Source[];
};
export type BundleImportResult = { applied: string[]; sourcesAdded: number };

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) message = body.error;
    } catch {
      /* keep status text */
    }
    throw new Error(message);
  }
  return res.json() as Promise<T>;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  return handleResponse<T>(res);
}

// Bypasses request()'s fixed JSON header — the browser must set its own
// multipart boundary in Content-Type for a FormData body.
async function requestUpload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, { method: "POST", body: form });
  return handleResponse<T>(res);
}

export const api = {
  overview: () => request<Overview>("/api/overview"),
  status: () => request<StatusResponse>("/api/status"),
  artifacts: () => request<Artifact[]>("/api/artifacts"),
  artifact: (id: string) => request<ArtifactDetail>(`/api/artifacts/${id}`),
  diff: (params: { artifact?: string; agent?: string }) => {
    const q = new URLSearchParams();
    if (params.artifact) q.set("artifact", params.artifact);
    if (params.agent) q.set("agent", params.agent);
    return request<Diff[]>(`/api/diff?${q}`);
  },
  syncPlan: () => request<SyncPlan>("/api/sync/plan", { method: "POST", body: "{}" }),
  syncApply: (token: string) =>
    request<{ applied: number }>("/api/sync/apply", {
      method: "POST",
      body: JSON.stringify({ token }),
    }),
  sources: () => request<Source[]>("/api/sources"),
  sourcesAdd: (input: { name?: string; url: string; ref?: string; subdir?: string }) =>
    request<SourceAddResult>("/api/sources/add", { method: "POST", body: JSON.stringify(input) }),
  sourcesCheck: () => request<SourceCheck[]>("/api/sources/check", { method: "POST", body: "{}" }),
  wizardSchema: () => request<WizardSection[]>("/api/wizard/schema"),
  wizardPresets: () => request<string[]>("/api/wizard/presets"),
  wizardPlan: (answers: Answers, preset?: string) =>
    request<{ create: string[]; skip: string[] }>("/api/wizard/plan", {
      method: "POST",
      body: JSON.stringify({ answers, preset }),
    }),
  wizardGenerate: (answers: Answers, preset?: string) =>
    request<{ created: string[]; skipped: string[] }>("/api/wizard/generate", {
      method: "POST",
      body: JSON.stringify({ answers, preset }),
    }),
  profiles: () => request<string[]>("/api/profiles"),
  profile: (name: string) => request<Answers>(`/api/profiles/${name}`),
  saveProfile: (name: string, answers: Answers) =>
    request<{ name: string }>("/api/profiles", {
      method: "POST",
      body: JSON.stringify({ name, answers }),
    }),
  deleteProfile: (name: string) =>
    request<{ removed: boolean }>(`/api/profiles/${name}`, { method: "DELETE" }),
  bundleImportPlan: (file: File) => {
    const form = new FormData();
    form.set("bundle", file);
    return requestUpload<BundleImportPlan>("/api/bundle/import/plan", form);
  },
  bundleImportApply: (token: string, overwrite: boolean) =>
    request<BundleImportResult>("/api/bundle/import/apply", {
      method: "POST",
      body: JSON.stringify({ token, overwrite }),
    }),
};
