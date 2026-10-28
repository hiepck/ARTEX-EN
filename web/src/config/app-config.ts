import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "Ops Console",
  version: packageJson.version,
  copyright: `© ${currentYear}, Ops Console.`,
  meta: {
    title: "Ops Console",
    description: "Internal operations console",
  },
};
