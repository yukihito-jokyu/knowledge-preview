import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function MarkdownRenderer({ children }: { children: string }) {
  return (
    <Markdown remarkPlugins={[remarkGfm]} skipHtml>
      {children}
    </Markdown>
  );
}
