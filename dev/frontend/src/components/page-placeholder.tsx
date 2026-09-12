export function PagePlaceholder({ title, description }: { title: string; description: string }) {
  return (
    <section>
      <h1 className="text-2xl font-bold">{title}</h1>
      <p className="mt-2 text-zinc-600">{description}</p>
    </section>
  );
}
