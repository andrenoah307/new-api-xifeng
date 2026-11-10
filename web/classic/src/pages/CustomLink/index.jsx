import React, { useContext, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ExternalLink, Loader2 } from "lucide-react";
import { StatusContext } from "../../context/Status";
import { parseCustomNavigationLinks } from "../../helpers/custom-navigation";

export default function CustomLink() {
  const { linkId } = useParams();
  const [state] = useContext(StatusContext);
  const { t } = useTranslation();
  const link = parseCustomNavigationLinks(state?.status?.CustomNavigationLinks).find(
    (item) => item.id === linkId && (item.header || item.sidebar),
  );
  const canEmbed = link?.mode === "iframe" && new URL(link.url).origin !== window.location.origin;
  const [frameLoading, setFrameLoading] = useState(true);
  useEffect(() => {
    setFrameLoading(true);
    const timer = window.setTimeout(() => setFrameLoading(false), 10000);
    return () => window.clearTimeout(timer);
  }, [link?.url]);
  return (
    <main className="mx-auto w-full max-w-screen-2xl px-4 py-4">
      {link ? (
        <>
          <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
            <h1 className="min-w-0 break-words text-xl font-semibold">{link.title}</h1>
            {canEmbed && frameLoading && <Loader2 className='size-4 animate-spin' role='status' aria-label={t('Loading...')} />}
            <a
              className="inline-flex items-center gap-2 text-sm underline"
              href={link.url}
              target="_blank"
              rel="noopener noreferrer"
            >
              <ExternalLink size={16} />
              {t("Open in new tab")}
            </a>
          </div>
          {canEmbed && (
            <iframe
              key={link.url}
              title={link.title}
              src={link.url}
              onLoad={() => setFrameLoading(false)}
              className="h-[75vh] min-h-96 w-full border-0 bg-white"
              sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-popups-to-escape-sandbox allow-downloads"
              referrerPolicy="no-referrer"
            />
          )}
        </>
      ) : (
        <p>{t("Not found")}</p>
      )}
    </main>
  );
}
