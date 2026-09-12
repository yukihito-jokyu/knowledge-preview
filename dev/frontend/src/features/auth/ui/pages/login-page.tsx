import { Button } from "@/components/ui/button";

export function LoginPage() {
  return (
    <section>
      <h1 className="text-2xl font-bold">ログイン</h1>
      <Button
        className="mt-4"
        onClick={() => {
          window.location.assign("/api/v1/auth/github");
        }}
      >
        GitHubでログイン
      </Button>
    </section>
  );
}
