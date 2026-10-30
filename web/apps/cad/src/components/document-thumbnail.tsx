import { useQuery, useQueryClient } from "@tanstack/react-query";
import { memo, useEffect, useRef, useState } from "react";
import { isMockMode, previewURL } from "../api/client";
import type { DocumentSummary } from "../types";
import { thumbnailRequests, type ThumbnailImage } from "./thumbnail-resource";

function ThumbnailImageView({ document, source }: { document: DocumentSummary; source: string }) {
  const client = useQueryClient();
  const queryKey = ["document-thumbnail", document.id, source];
  const resource = useQuery({
    queryKey,
    queryFn: async ({ signal }) => {
      const previous = client.getQueryData<ThumbnailImage & { attempts: number }>(queryKey);
      const image = await thumbnailRequests.load(source, signal);
      return { ...image, attempts: image.pending ? (previous?.attempts ?? 0) + 1 : 0 };
    },
    // FOLLOW_HEAD can change Product geometry without changing its own Revision.
    // Show cached blobs immediately and revalidate on every visible mount.
    staleTime: 0, gcTime: 120_000, retry: 1,
    refetchInterval: query => {
      const image = query.state.data;
      return image?.pending && image.attempts <= 12 ? Math.min(5000, 750 * 2 ** Math.min(image.attempts, 3)) : false;
    },
  });
  const [image, setImage] = useState<{ blob: Blob; url: string }>();
  const blob = resource.data?.blob;
  useEffect(() => {
    if (!blob) return;
    const url = URL.createObjectURL(blob);
    setImage({ blob, url });
    return () => URL.revokeObjectURL(url);
  }, [blob]);
  return image
    ? <img decoding="async" src={image.url} alt={`${document.name} 缩略图`} onError={() => setImage(undefined)} />
    : <span className="empty-scene-preview" aria-label="空场景" />;
}

/** Query owns reusable blobs; each visible mounted view owns its object URL. */
export const DocumentThumbnail = memo(function DocumentThumbnail({ document }: { document: DocumentSummary }) {
  const host = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(false);
  const source = previewURL(document.id, document.versionId);
  useEffect(() => {
    if (isMockMode || !source || !host.current) return;
    const observer = new IntersectionObserver(entries => {
      if (entries.some(entry => entry.isIntersecting)) { observer.disconnect(); setVisible(true); }
    }, { rootMargin: "160px" });
    observer.observe(host.current);
    return () => observer.disconnect();
  }, [source]);
  return <span ref={host} className="document-thumbnail">
    {visible && source ? <ThumbnailImageView key={source} document={document} source={source} /> : <span className="empty-scene-preview" aria-label="空场景" />}
  </span>;
});
