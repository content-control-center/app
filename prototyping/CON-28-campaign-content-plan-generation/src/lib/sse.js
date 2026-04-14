export function parseSSEBlock(rawBlock) {
  const lines = rawBlock.split("\n");
  let eventName = "message";
  const dataLines = [];

  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventName = line.slice(6).trim();
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  const rawData = dataLines.join("\n");
  if (!rawData) return { eventName, data: null };

  try {
    return { eventName, data: JSON.parse(rawData) };
  } catch {
    return { eventName, data: rawData };
  }
}

export async function consumeSSEStream(reader, onEvent) {
  const decoder = new TextDecoder();
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done }).replaceAll("\r", "");

    let delimiterIndex = buffer.indexOf("\n\n");
    while (delimiterIndex >= 0) {
      const block = buffer.slice(0, delimiterIndex).trim();
      buffer = buffer.slice(delimiterIndex + 2);
      if (block) onEvent(parseSSEBlock(block));
      delimiterIndex = buffer.indexOf("\n\n");
    }

    if (done) break;
  }

  const rest = buffer.trim();
  if (rest) onEvent(parseSSEBlock(rest));
}
