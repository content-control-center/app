import { useEffect, useState } from "react";

function App() {
  const [status, setStatus] = useState<string>("checking…");

  useEffect(() => {
    fetch("/api/health")
      .then((r) => r.json())
      .then((d) => setStatus(d.status ?? "unknown"))
      .catch(() => setStatus("unreachable"));
  }, []);

  return (
    <main>
      <h1>Content Control Center application</h1>
      <p>
        API status: <strong>{status}</strong>
      </p>
    </main>
  );
}

export default App;
