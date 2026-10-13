// plural writes a count with the singular or plural noun that agrees with it
export const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
