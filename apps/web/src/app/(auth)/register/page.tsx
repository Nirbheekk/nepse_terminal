import type { Metadata } from "next";
import { AuthCard } from "@/components/terminal/auth-card";

export const metadata: Metadata = { title: "Create account · NEPSE Terminal" };

export default function RegisterPage() {
  return <AuthCard mode="register" />;
}
