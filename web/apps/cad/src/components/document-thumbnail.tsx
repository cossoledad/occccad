import { ApartmentOutlined, BuildOutlined } from "@ant-design/icons";
import { useEffect, useState } from "react";
import { isMockMode, previewURL } from "../api/client";
import type { DocumentSummary } from "../types";

export function DocumentThumbnail({ document }: { document: DocumentSummary }) {
  const [failed, setFailed] = useState(false);
  const source = previewURL(document.id, document.versionId);
  useEffect(() => setFailed(false), [document.id, document.versionId]);
  if (isMockMode || !source || failed) {
    return <span className={`document-thumbnail fallback ${document.type.toLowerCase()}`}>
      {document.type === "PART" ? <BuildOutlined /> : <ApartmentOutlined />}
    </span>;
  }
  return <span className="document-thumbnail">
    <img src={`${source}&attempt=0`} alt={`${document.name} 缩略图`} onError={() => setFailed(true)} />
  </span>;
}
