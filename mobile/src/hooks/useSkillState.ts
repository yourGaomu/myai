import { useCallback, useState } from "react";

import type { SkillSummary } from "../protocol";

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

