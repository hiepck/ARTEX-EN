import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "Penred",
  version: packageJson.version,
  copyright: `© ${currentYear}, Penred.`,
  meta: {
    title: "Penred — Autonomous penetration testing console",
    description: "Console for the LLM-driven autonomous penetration testing system",
  },
};
