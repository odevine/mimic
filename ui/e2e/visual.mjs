import path from "node:path";

// Where the baselines are. They live with the other test assets, and a developer
// who wants to look at the comparison on a machine that is not the one that makes
// the baselines sets MIMIC_VISUAL_LOCAL, which keeps a private set in .scratch
export const snapshotDir = path.resolve(import.meta.dirname, process.env.MIMIC_VISUAL_LOCAL ? ".scratch/visual" : "../testassets/visual");

// The visual project runs where the baselines are made, which is Linux, or
// anywhere with a private set
export const visualEnabled = process.platform === "linux" || !!process.env.MIMIC_VISUAL_LOCAL;
