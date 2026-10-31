import type { Metadata } from "next";
import { AuthCard } from "@/components/terminal/auth-card";

export const metadata: Metadata = { title: "Sign in · NEPSE Terminal" };

export default function LoginPage() {
  return <AuthCard mode="login" />;
}
