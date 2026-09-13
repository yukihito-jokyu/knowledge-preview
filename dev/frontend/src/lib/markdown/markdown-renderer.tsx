import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

export function MarkdownRenderer({
  children,
  components,
}: {
  children: string;
  components?: Components;
}) {
  return (
    <Markdown remarkPlugins={[remarkGfm]} skipHtml components={components}>
      {children}
    </Markdown>
  );
}
