import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const projectRoot = path.resolve(scriptDir, '..');
const testDir = path.join(projectRoot, 'tests');

function findTestFiles(dir) {
  const entries = fs.readdirSync(dir, { withFileTypes: true });
  const files = [];

  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...findTestFiles(fullPath));
      continue;
    }

    if (entry.isFile() && entry.name.endsWith('.test.ts')) {
      files.push(fullPath);
    }
  }

  return files.sort();
}

function resolveTestFiles(cliArgs) {
  const allTestFiles = findTestFiles(testDir);
  if (cliArgs.length === 0) {
    return allTestFiles;
  }

  const requested = cliArgs.map((inputPath) => {
    const absolutePath = path.isAbsolute(inputPath) ? inputPath : path.resolve(projectRoot, inputPath);
    if (!fs.existsSync(absolutePath)) {
      console.error(`Test file not found: ${inputPath}`);
      process.exit(1);
    }
    return absolutePath;
  });

  return requested.filter((filePath) => filePath.endsWith('.test.ts')).sort();
}


function readPrismaDefaultDatabaseUrl() {
  const prismaConfigPath = path.join(projectRoot, 'prisma.config.ts');
  const configSource = fs.readFileSync(prismaConfigPath, 'utf8');
  const match = configSource.match(/url:\s*process\.env\.DATABASE_URL\s*\?\?\s*'([^']+)'/);

  if (!match) {
    throw new Error('Could not determine default DATABASE_URL from prisma.config.ts');
  }

  return match[1];
}

function resolveTestDatabaseUrl(inputUrl) {
  const url = new URL(inputUrl);
  const databaseName = url.pathname.split('/').filter(Boolean).pop();

  if (databaseName === 'live_commerce_test') {
    return url.toString();
  }

  if (databaseName === 'massage_directory') {
    url.pathname = `${url.pathname.slice(0, url.pathname.lastIndexOf('/') + 1)}live_commerce_test`;
    return url.toString();
  }

  throw new Error(`Refusing to run tests against non-test database: ${databaseName ?? '(unknown)'}`);
}

function createTestEnv() {
  const env = { ...process.env };
  const sourceUrl = env.DATABASE_URL ?? readPrismaDefaultDatabaseUrl();
  env.DATABASE_URL = resolveTestDatabaseUrl(sourceUrl);
  return env;
}

function getNodeRunner() {
  const major = Number.parseInt(process.versions.node.split('.')[0] ?? '0', 10);
  if (major >= 22) {
    return { command: process.execPath, baseArgs: [] };
  }

  return {
    command: 'npx',
    baseArgs: ['-y', 'node@22'],
  };
}

function runCommand(command, args, env) {
  return spawnSync(command, args, {
    cwd: projectRoot,
    stdio: 'inherit',
    env,
  });
}

const nodeRunner = getNodeRunner();
const testFiles = resolveTestFiles(process.argv.slice(2));
if (testFiles.length === 0) {
  console.error('No test files found under tests/.');
  process.exit(1);
}

const testEnv = createTestEnv();
const seedResult = runCommand(
  nodeRunner.command,
  [...nodeRunner.baseArgs, '--import', 'tsx', '--experimental-transform-types', 'prisma/seed.ts'],
  testEnv,
);
if ((seedResult.status ?? 1) !== 0) {
  process.exit(seedResult.status ?? 1);
}

let failed = false;

for (const filePath of testFiles) {
  const relativePath = path.relative(projectRoot, filePath).split(path.sep).join('/');
  console.log(`
=== Running ${relativePath} ===`);

  const result = runCommand(
    nodeRunner.command,
    [
      ...nodeRunner.baseArgs,
      '--disable-warning=ExperimentalWarning',
      '--disable-warning=DeprecationWarning',
      '--experimental-transform-types',
      '--import',
      'tsx',
      '--import',
      pathToFileURL(path.join(projectRoot, 'scripts', 'register-alias.mjs')).href,
      '--test',
      relativePath,
    ],
    testEnv,
  );

  if ((result.status ?? 1) !== 0) {
    failed = true;
  }
}

process.exit(failed ? 1 : 0);
