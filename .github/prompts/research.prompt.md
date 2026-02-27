````prompt
---
agent: agent
---

# Research & Documentation Gap Analysis Prompt

## Purpose
This prompt guides a structured Q&A session to explore and complete documentation for any feature or architectural component in **CodeValdWork** through **one question at a time**.

---

## 🔄 MANDATORY REFACTOR WORKFLOW (ENFORCE BEFORE ANY RESEARCH SESSION)

**BEFORE starting any research or writing new task documentation:**

### Step 1: CHECK File Size
```bash
wc -l documentation/3-SofwareDevelopment/mvp-details/{topic-file}.md
```

### Step 2: IF >500 lines OR individual WORK-XXX.md files exist:

**a. CREATE folder structure:**
```bash
documentation/3-SofwareDevelopment/mvp-details/{domain-name}/
├── README.md              # Domain overview, architecture, task index (MAX 300 lines)
├── {topic-1}.md           # Topic-based grouping of related tasks (MAX 500 lines)
└── {topic-2}.md
```

**b. SPLIT content by TOPIC (NOT by task ID)**

**c. MOVE examples** → `examples/` subfolder

### Step 3: ONLY THEN add new task content to appropriate topic file

---

## 🛑 STOP CONDITIONS (Do NOT proceed until fixed)

- ❌ **Domain file exceeds 500 lines** → **MUST refactor first**
- ❌ **README.md exceeds 300 lines** → **MUST split content**
- ❌ **Individual `WORK-XXX.md` files exist** → **MUST consolidate by topic**

---

## Instructions for AI Assistant

Conduct a comprehensive documentation gap analysis through **iterative single-question exploration**. Ask ONE question at a time, wait for the response, then decide whether to:

- 🔍 **DEEPER**: Examine the area more closely
- 📝 **NOTE**: Record an issue/gap for later action
- ➡️ **NEXT**: Move to the next consistency check area
- 📊 **REVIEW**: Summarize findings and determine next steps

### Session Initiation

When starting a research session:
1. State the scope — which feature or task are we researching?
2. Check current file sizes and structure
3. Ask the first targeted question
4. Wait for user input before proceeding
````
