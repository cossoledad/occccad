import { ApartmentOutlined, BuildOutlined } from "@ant-design/icons";
import { useState } from "react";
import { isMockMode, previewURL } from "../api/client";
import type { DocumentSummary } from "../types";

export function DocumentThumbnail({ document }: { document: DocumentSummary }) {
  const [attempt, setAttempt] = useState(0);
  const source = previewURL(document.id, document.versionId);
  if (isMockMode || !source || attempt > 3) {
    return <span className={`document-thumbnail fallback ${document.type.toLowerCase()}`}>
      {document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />}
      <i /><i /><i />
    </span>;
  }
  return <span className="document-thumbnail">
    <img src={`${source}&attempt=${attempt}`} alt={`${document.name} 缩略图`}
      onError={() => window.setTimeout(() => setAttempt((value) => value + 1), 700 * (attempt + 1))} />
  </span>;
}
