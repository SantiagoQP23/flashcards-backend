-- CreateEnum
CREATE TYPE "Accent" AS ENUM ('US', 'UK');

-- CreateEnum
CREATE TYPE "AudioSource" AS ENUM ('PROVIDER', 'KOKORO');

-- CreateEnum
CREATE TYPE "CardState" AS ENUM ('NEW', 'LEARNING', 'REVIEW', 'RELEARNING');

-- CreateEnum
CREATE TYPE "Rating" AS ENUM ('AGAIN', 'HARD', 'GOOD', 'EASY');

-- CreateTable
CREATE TABLE "User" (
    "id" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "passwordHash" TEXT,
    "googleSub" TEXT,
    "displayName" TEXT,
    "timezone" TEXT NOT NULL DEFAULT 'UTC',
    "preferredAccent" "Accent" NOT NULL DEFAULT 'US',
    "dailyNewLimit" INTEGER NOT NULL DEFAULT 20,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "User_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "RefreshToken" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "tokenHash" TEXT NOT NULL,
    "expiresAt" TIMESTAMP(3) NOT NULL,
    "revokedAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "RefreshToken_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "DictionaryWord" (
    "id" TEXT NOT NULL,
    "language" TEXT NOT NULL DEFAULT 'en',
    "lemma" TEXT NOT NULL,
    "phoneticUs" TEXT,
    "phoneticUk" TEXT,
    "source" TEXT NOT NULL,
    "sourceUrls" TEXT[],
    "fetchedAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "DictionaryWord_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Definition" (
    "id" TEXT NOT NULL,
    "wordId" TEXT NOT NULL,
    "partOfSpeech" TEXT NOT NULL,
    "text" TEXT NOT NULL,
    "example" TEXT,
    "synonyms" TEXT[],
    "antonyms" TEXT[],
    "position" INTEGER NOT NULL,

    CONSTRAINT "Definition_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "WordAudio" (
    "id" TEXT NOT NULL,
    "wordId" TEXT NOT NULL,
    "accent" "Accent" NOT NULL,
    "source" "AudioSource" NOT NULL,
    "url" TEXT NOT NULL,
    "voice" TEXT,

    CONSTRAINT "WordAudio_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "SearchHistory" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "query" TEXT NOT NULL,
    "wordId" TEXT,
    "searchedAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "SearchHistory_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Deck" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "deletedAt" TIMESTAMP(3),

    CONSTRAINT "Deck_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "VocabularyEntry" (
    "id" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "deckId" TEXT NOT NULL,
    "wordId" TEXT NOT NULL,
    "definitionId" TEXT NOT NULL,
    "note" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,
    "deletedAt" TIMESTAMP(3),

    CONSTRAINT "VocabularyEntry_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Flashcard" (
    "id" TEXT NOT NULL,
    "entryId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "state" "CardState" NOT NULL DEFAULT 'NEW',
    "due" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "stability" DOUBLE PRECISION NOT NULL DEFAULT 0,
    "difficulty" DOUBLE PRECISION NOT NULL DEFAULT 0,
    "elapsedDays" INTEGER NOT NULL DEFAULT 0,
    "scheduledDays" INTEGER NOT NULL DEFAULT 0,
    "reps" INTEGER NOT NULL DEFAULT 0,
    "lapses" INTEGER NOT NULL DEFAULT 0,
    "lastReview" TIMESTAMP(3),

    CONSTRAINT "Flashcard_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Review" (
    "id" TEXT NOT NULL,
    "clientReviewId" TEXT NOT NULL,
    "flashcardId" TEXT NOT NULL,
    "userId" TEXT NOT NULL,
    "rating" "Rating" NOT NULL,
    "stateBefore" "CardState" NOT NULL,
    "reviewedAt" TIMESTAMP(3) NOT NULL,
    "durationMs" INTEGER,
    "scheduledDays" INTEGER NOT NULL,
    "dueAfter" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Review_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "User_email_key" ON "User"("email");

-- CreateIndex
CREATE UNIQUE INDEX "User_googleSub_key" ON "User"("googleSub");

-- CreateIndex
CREATE UNIQUE INDEX "RefreshToken_tokenHash_key" ON "RefreshToken"("tokenHash");

-- CreateIndex
CREATE INDEX "RefreshToken_userId_idx" ON "RefreshToken"("userId");

-- CreateIndex
CREATE UNIQUE INDEX "DictionaryWord_language_lemma_key" ON "DictionaryWord"("language", "lemma");

-- CreateIndex
CREATE INDEX "Definition_wordId_idx" ON "Definition"("wordId");

-- CreateIndex
CREATE UNIQUE INDEX "WordAudio_wordId_accent_source_key" ON "WordAudio"("wordId", "accent", "source");

-- CreateIndex
CREATE INDEX "SearchHistory_userId_searchedAt_idx" ON "SearchHistory"("userId", "searchedAt");

-- CreateIndex
CREATE INDEX "Deck_userId_idx" ON "Deck"("userId");

-- CreateIndex
CREATE INDEX "VocabularyEntry_userId_deckId_idx" ON "VocabularyEntry"("userId", "deckId");

-- CreateIndex
CREATE UNIQUE INDEX "VocabularyEntry_userId_definitionId_key" ON "VocabularyEntry"("userId", "definitionId");

-- CreateIndex
CREATE UNIQUE INDEX "Flashcard_entryId_key" ON "Flashcard"("entryId");

-- CreateIndex
CREATE INDEX "Flashcard_userId_due_idx" ON "Flashcard"("userId", "due");

-- CreateIndex
CREATE UNIQUE INDEX "Review_clientReviewId_key" ON "Review"("clientReviewId");

-- CreateIndex
CREATE INDEX "Review_userId_reviewedAt_idx" ON "Review"("userId", "reviewedAt");

-- AddForeignKey
ALTER TABLE "RefreshToken" ADD CONSTRAINT "RefreshToken_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Definition" ADD CONSTRAINT "Definition_wordId_fkey" FOREIGN KEY ("wordId") REFERENCES "DictionaryWord"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "WordAudio" ADD CONSTRAINT "WordAudio_wordId_fkey" FOREIGN KEY ("wordId") REFERENCES "DictionaryWord"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SearchHistory" ADD CONSTRAINT "SearchHistory_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "SearchHistory" ADD CONSTRAINT "SearchHistory_wordId_fkey" FOREIGN KEY ("wordId") REFERENCES "DictionaryWord"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Deck" ADD CONSTRAINT "Deck_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VocabularyEntry" ADD CONSTRAINT "VocabularyEntry_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VocabularyEntry" ADD CONSTRAINT "VocabularyEntry_deckId_fkey" FOREIGN KEY ("deckId") REFERENCES "Deck"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VocabularyEntry" ADD CONSTRAINT "VocabularyEntry_wordId_fkey" FOREIGN KEY ("wordId") REFERENCES "DictionaryWord"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "VocabularyEntry" ADD CONSTRAINT "VocabularyEntry_definitionId_fkey" FOREIGN KEY ("definitionId") REFERENCES "Definition"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Flashcard" ADD CONSTRAINT "Flashcard_entryId_fkey" FOREIGN KEY ("entryId") REFERENCES "VocabularyEntry"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Flashcard" ADD CONSTRAINT "Flashcard_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Review" ADD CONSTRAINT "Review_flashcardId_fkey" FOREIGN KEY ("flashcardId") REFERENCES "Flashcard"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Review" ADD CONSTRAINT "Review_userId_fkey" FOREIGN KEY ("userId") REFERENCES "User"("id") ON DELETE CASCADE ON UPDATE CASCADE;
