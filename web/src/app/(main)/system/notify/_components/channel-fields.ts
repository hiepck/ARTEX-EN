// The channel field table and the tools for parsing a configuration value.
//
// It is split away from the page because this is **data** rather than a view: it describes which fields each channel has,
// which control each one needs, and the two-way conversion between the form text and the configuration value (JSON).
// With it in a file of its own, adding a channel only means touching this file and the page itself stays unchanged.
// The display names and short descriptions of the channel types. They live in the frontend because they affect wording only and the backend does not need to know them.
export const KIND_LABEL: Record<string, string> = {
  dingtalk: "DingTalk",
  feishu: "Feishu",
  wecom: "WeCom",
  webhook: "A generic webhook",
  telegram: "Telegram",
  email: "Email",
};

// The configuration field definitions of each channel.
//
// A frontend field table is kept here deliberately rather than having the backend serve a schema: the backend only handles
// validation (required fields and formats), while the UI needs a layout and control types -- the two are not concerned with the same thing.
// The one coupling point is secret_keys -- which fields render as a password box comes from the backend,
// because only the channel implementation knows which values count as credentials (WeCom's whole webhook is a credential,
// while DingTalk's is just one secret inside it). A missing entry here when a channel is added only leaves the form blank,
// it never fails silently (hasFields below says so).
export type FieldKind = "text" | "password" | "number" | "select" | "textarea" | "switch" | "kv" | "list";
export interface FieldDef {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  help?: string;
  options?: { value: string; label: string }[];
}
export const CHANNEL_FIELDS: Record<string, FieldDef[]> = {
  dingtalk: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...",
    },
    {
      key: "secret",
      label: "Signing secret",
      kind: "password",
      help: "Fill it in when the bot's security setting is \"signed\"; leave it empty for \"custom keywords\" or with no security setting enabled",
    },
  ],
  feishu: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...",
    },
    { key: "secret", label: "Signature verification key", kind: "password", help: "Fill it in when the bot has \"signature verification\" enabled, otherwise leave it empty" },
  ],
  wecom: [
    {
      key: "webhook",
      label: "Webhook address",
      kind: "text",
      placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...",
    },
  ],
  webhook: [
    { key: "url", label: "Target URL", kind: "text", placeholder: "https://your-endpoint.example.com/hook" },
    {
      key: "method",
      label: "Request method",
      kind: "select",
      options: [
        { value: "POST", label: "POST (with a body)" },
        { value: "PUT", label: "PUT (with a body)" },
        { value: "PATCH", label: "PATCH (with a body)" },
        { value: "GET", label: "GET (no body)" },
      ],
    },
    { key: "headers", label: "Custom request headers", kind: "kv", help: "One KEY=VALUE per line, such as Authorization=Bearer xxx" },
    {
      key: "body_template",
      label: "Request body template",
      kind: "textarea",
      help:
        "Leave it empty to use the built-in default template. The variables: {{.Title}} {{.Batch}} {{.Count}} {{.HomeURL}} {{.SentAt}}, " +
        "plus .Name/.VulnClass/.Severity/.Summary/.Assets/.DetailURL/.StatusLabel under range .Items. " +
        "Insert a string with {{json .Xxx}} rather than {{.Xxx}}, or a quote in a title will break the JSON.",
    },
  ],
  telegram: [
    { key: "bot_token", label: "Bot Token", kind: "password", placeholder: "123456:ABC-DEF..." },
    { key: "chat_id", label: "Chat ID", kind: "text", placeholder: "-1001234567890" },
    {
      key: "base_url",
      label: "API address",
      kind: "text",
      placeholder: "https://api.telegram.org",
      help: "Leave it empty for the official address; fill it in when reverse-proxying a self-hosted Bot API",
    },
  ],
  email: [
    { key: "host", label: "SMTP server", kind: "text", placeholder: "smtp.example.com" },
    {
      key: "port",
      label: "Port",
      kind: "number",
      placeholder: "587",
      help: "587 uses STARTTLS; for 465 turn \"implicit TLS\" on",
    },
    { key: "username", label: "Username", kind: "text" },
    { key: "password", label: "Password / app password", kind: "password" },
    { key: "from", label: "From", kind: "text", placeholder: "noreply@example.com" },
    { key: "to", label: "To", kind: "list", help: "Separate several addresses with commas" },
    { key: "tls", label: "Implicit TLS", kind: "switch", help: "On for port 465; keep it off for 587 (which negotiates STARTTLS automatically)" },
  ],
};

export const SEVERITY_OPTIONS = [
  { value: "", label: "No limit" },
  { value: "low", label: "Low and above" },
  { value: "medium", label: "Medium and above" },
  { value: "high", label: "High and above" },
  { value: "critical", label: "Critical only" },
];

export type ChannelForm = {
  name: string;
  kind: string;
  mode: "realtime" | "digest";
  enabled: boolean;
  ratePerMin: string;
  config: Record<string, unknown>;
  minSeverity: string;
  includeText: string;
  excludeText: string;
  taskIDsText: string;
  assetIDsText: string;
  onStatusChange: boolean;
};

export const emptyForm = (kind: string): ChannelForm => ({
  name: "",
  kind,
  mode: "realtime",
  enabled: true,
  ratePerMin: "",
  config: {},
  minSeverity: "",
  includeText: "",
  excludeText: "",
  taskIDsText: "",
  assetIDsText: "",
  onStatusChange: false,
});

// parseKV parses a textarea of "one KEY=VALUE per line".
export function parseKV(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const t = line.trim();
    if (!t) continue;
    const i = t.indexOf("=");
    if (i > 0) out[t.slice(0, i).trim()] = t.slice(i + 1).trim();
  }
  return out;
}
// parseIDs parses a comma- or whitespace-separated id list.
export function parseIDs(text: string): number[] {
  return text
    // The full-width comma stays in the separator class on purpose: it is what a Chinese IME types.
    .split(/[\s,，]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    .map((s) => Number(s))
    .filter((n) => Number.isFinite(n) && n > 0);
}
// parseKeywords parses a line- or comma-separated keyword list (a finding type name may contain spaces, so it splits on a newline or a comma).
export function parseKeywords(text: string): string[] {
  return text
    // The full-width comma stays in the separator class on purpose: it is what a Chinese IME types.
    .split(/[\n,，]+/)
    .map((s) => s.trim())
    .filter(Boolean);
}
