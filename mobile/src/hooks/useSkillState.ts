import { useCallback, useState } from "react";

import type { SkillSummary } from "../protocol";

// 保存 Skill 列表、根目录和最近一次 reload 提示。
export function useSkillState() {
  const [skillRoot, setSkillRoot] = useState("");
  const [skillMessage, setSkillMessage] = useState("");
  const [skills, setSkills] = useState<SkillSummary[]>([]);

  const clearSkills = useCallback(() => {
    setSkillRoot("");
    setSkillMessage("");
    setSkills([]);
  }, []);

  return {
    clearSkills,
    setSkillMessage,
    setSkillRoot,
    setSkills,
    skillMessage,
    skillRoot,
    skills,
  };
}
