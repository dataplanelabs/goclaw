import { describe, expect, it } from "vitest";

import { parseRichContent } from "./rich-content-parser";

describe("parseRichContent — media tag URL extraction", () => {
  it("bare <media:image> → media block without url (chat-thread case)", () => {
    const blocks = parseRichContent("<media:image>");
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toEqual({ type: "media", mediaType: "image" });
  });

  it("<media:image url='https://...'> → media block with url (Team Analytics case)", () => {
    const blocks = parseRichContent('<media:image url="https://zalo-api.zadn.vn/api/emoticon/oasticker?eid=1442">');
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toEqual({
      type: "media",
      mediaType: "image",
      url: "https://zalo-api.zadn.vn/api/emoticon/oasticker?eid=1442",
    });
  });

  it("rejects javascript: URL (XSS safety)", () => {
    const blocks = parseRichContent('<media:image url="javascript:alert(1)">');
    expect(blocks[0]).toEqual({ type: "media", mediaType: "image" });
  });

  it("rejects data: URL", () => {
    const blocks = parseRichContent('<media:image url="data:image/png;base64,iVBOR">');
    expect(blocks[0]).toEqual({ type: "media", mediaType: "image" });
  });

  it("rejects file: URL", () => {
    const blocks = parseRichContent('<media:image url="file:///etc/passwd">');
    expect(blocks[0]).toEqual({ type: "media", mediaType: "image" });
  });

  it("accepts http:// URL", () => {
    const blocks = parseRichContent('<media:image url="http://example.com/x.jpg">');
    expect(blocks[0]).toEqual({ type: "media", mediaType: "image", url: "http://example.com/x.jpg" });
  });

  it("text + <media:image url=...> + text — media block extracted separately from markdown", () => {
    const blocks = parseRichContent('before <media:image url="https://x/a.jpg"> after');
    expect(blocks.find((b) => b.type === "media")).toBeDefined();
    expect(blocks.find((b) => b.type === "markdown")).toBeDefined();
  });
});
