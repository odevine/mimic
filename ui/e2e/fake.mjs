// The control paths of the fake upstream the app talks to. A test changes what
// Scryfall, GitHub and the template catalog answer, and what the app asked for
export class Fake {
  constructor(port) {
    this.base = `http://127.0.0.1:${port}/__fake`;
  }

  async #send(method, path, body) {
    const res = await fetch(this.base + path, { method, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!res.ok) throw new Error(`${method} ${path}: ${res.status} ${await res.text()}`);
    return res.status === 204 ? undefined : res.json();
  }

  // release offers a ui release. Without one the app is up to date
  release(version, notes = "") {
    return this.#send("PUT", "/release", { version, notes });
  }

  // catalog lists template bundles, as [{ name, version }]. Without it the
  // catalog is empty
  catalog(templates) {
    return this.#send("PUT", "/catalog", { templates });
  }

  // fail makes a host answer every request with a status of 400 or more
  fail(host, status = 503) {
    return this.#send("PUT", "/fail", { host, status });
  }

  // delay makes a host wait ms before each answer
  delay(host, ms) {
    return this.#send("PUT", "/delay", { host, ms });
  }

  // requests lists what the app has asked for, as "host path"
  requests() {
    return this.#send("GET", "/requests");
  }

  // reset puts the fake back to its start
  async reset() {
    for (const path of ["/release", "/catalog", "/fail", "/delay", "/requests"]) await this.#send("DELETE", path);
  }
}
