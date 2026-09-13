import { MarkdownRenderer } from "@/lib/markdown/markdown-renderer";

function getHeadings(source: string) {
  let fence = "";

  return source.split("\n").flatMap((line, index) => {
    const delimiter = /^\s{0,3}(`{3,}|~{3,})/.exec(line)?.[1];
    if (delimiter) {
      fence =
        fence && delimiter.startsWith(fence[0] ?? "") && delimiter.length >= fence.length
          ? ""
          : fence || delimiter;
      return [];
    }
    if (fence) return [];
    const heading = /^ {0,3}(#{1,3})\s+(.+?)\s*#*\s*$/.exec(line);
    return heading ? [{ id: `heading-${index + 1}`, title: heading[2] }] : [];
  });
}

export function MarkdownPreview({ source }: { source: string }) {
  const headings = getHeadings(source);
  return (
    <div className="flex flex-col gap-5 2xl:flex-row">
      <article className="knowledge-markdown min-w-0 flex-1">
        <MarkdownRenderer
          components={{
            h1: ({ node, children }) => (
              <h1 id={`heading-${node?.position?.start.line}`}>{children}</h1>
            ),
            h2: ({ node, children }) => (
              <h2 id={`heading-${node?.position?.start.line}`}>{children}</h2>
            ),
            h3: ({ node, children }) => (
              <h3 id={`heading-${node?.position?.start.line}`}>{children}</h3>
            ),
          }}
        >
          {source}
        </MarkdownRenderer>
      </article>
      {headings.length > 0 && (
        <nav aria-label="このページ" className="text-sm 2xl:w-36">
          <h2 className="mb-2 font-medium">このページ</h2>
          <ul className="space-y-2">
            {headings.map((heading) => (
              <li key={heading.id}>
                <a className="text-muted-foreground hover:text-primary" href={`#${heading.id}`}>
                  {heading.title}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      )}
    </div>
  );
}
