// Stands in for the Wails runtime: Events.On records handlers so a test can emit,
// and Call.ByName answers with whatever the test sets in replies
export const handlers = {};
export const replies = {};
export const Events = { On: (name, cb) => { handlers[name] = cb; return () => {}; } };
export const Call = { ByName: async (name) => (name in replies ? replies[name] : { events: [], finished: false }) };
