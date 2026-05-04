import { createClient } from "@supabase/supabase-js";

// Single browser-side client. The anon key is safe to expose;
// row-level-security is what actually protects the data.
export const supabase = createClient(
  process.env.NEXT_PUBLIC_SUPABASE_URL!,
  process.env.NEXT_PUBLIC_SUPABASE_ANON_KEY!,
);

export type Todo = {
  id: string;
  user_id: string;
  title: string;
  done: boolean;
  created_at: string;
};
