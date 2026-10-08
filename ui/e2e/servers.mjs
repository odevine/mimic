// The test servers. The main one serves nearly every test and is put back to its
// defaults after each. The isolated ones serve the tests that change what the app
// keeps for good, such as installing a template, and there is one for each
// browser, since the second browser would otherwise start from what the first
// left. Each has its own scratch folder and its own fake upstream, whose control
// port a test reaches by the name of the server its project uses
export const servers = {
  main: { port: 4173, fake: 4180, home: ".scratch/home" },
  "chromium-isolated": { port: 4174, fake: 4181, home: ".scratch/chromium-isolated-home" },
  "webkit-isolated": { port: 4175, fake: 4182, home: ".scratch/webkit-isolated-home" },
};
